package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"sni-gateway/internal/api"
	"sni-gateway/internal/catalog"
	"sni-gateway/internal/config"
	"sni-gateway/internal/db"
	"sni-gateway/internal/docker"
	"sni-gateway/internal/gateway"
	"sni-gateway/internal/services"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := catalog.LoadCache(cfg.ProviderCatalogPath); err != nil {
		log.Printf("Provider catalog cache unavailable; using bundled data: %v", err)
	}
	updateInterval, err := time.ParseDuration(cfg.ProviderCatalogUpdateInterval)
	if err != nil {
		log.Fatalf("Invalid PROVIDER_CATALOG_UPDATE_INTERVAL: %v", err)
	}
	idleTimeout, err := time.ParseDuration(cfg.TLSIdleTimeout)
	if err != nil || idleTimeout <= 0 {
		log.Fatalf("Invalid TLS_IDLE_TIMEOUT: %q", cfg.TLSIdleTimeout)
	}
	tlsMaxConnections := gateway.DefaultMaxConnections
	if cfg.TLSMaxConnections != "" {
		tlsMaxConnections, err = strconv.Atoi(cfg.TLSMaxConnections)
		if err != nil || tlsMaxConnections <= 0 {
			log.Fatalf("Invalid TLS_MAX_CONNECTIONS %q. Set it to a positive integer.", cfg.TLSMaxConnections)
		}
	}
	ddnsInterval, err := time.ParseDuration(cfg.WireguardDDNSCheckInterval)
	if err != nil || ddnsInterval < 0 {
		log.Fatalf("Invalid WIREGUARD_DDNS_CHECK_INTERVAL: %q", cfg.WireguardDDNSCheckInterval)
	}

	// Wait for Postgres
	for i := 0; i < 30; i++ {
		if err := db.Init(cfg.DatabaseURL); err == nil {
			break
		} else {
			log.Printf("Waiting for database... (%d/30): %v", i+1, err)
			time.Sleep(2 * time.Second)
		}
		if i == 29 {
			log.Fatal("Could not connect to database after 30 attempts")
		}
	}

	// Docker client (optional — gracefully degrade if not available)
	dockerClient, err := docker.NewClient(cfg.DockerSocket)
	if err != nil {
		log.Printf("Warning: Docker client unavailable: %v", err)
		log.Printf("Container management features will be disabled.")
	}

	exitMgr := services.NewExitManager(dockerClient)

	cfgGen, err := services.NewRoutePublisher(cfg.RouteSnapshotPath)
	if err != nil {
		log.Fatalf("Load published routes: %v", err)
	}
	subMgr := services.NewSubscriptionManager(cfgGen)
	restartScheduler := services.NewExitRestartScheduler(exitMgr, cfg.ExitRestartCron)

	subMgr.Start()
	defer subMgr.Stop()
	restartScheduler.Start()
	defer restartScheduler.Stop()

	addr := ":" + cfg.Port
	log.Printf("SNI Gateway backend starting on %s", addr)

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	catalog.StartUpdater(ctx, cfg.ProviderCatalogPath, updateInterval)
	ingress := &gateway.Server{
		Router: cfgGen, HTTPAddr: ":80", TLSAddr: ":443",
		IdleTimeout: idleTimeout, MaxConnections: tlsMaxConnections,
	}
	if err := ingress.Start(ctx); err != nil {
		log.Fatalf("Start SNI ingress: %v", err)
	}
	defer ingress.Close()
	ddnsDone := make(chan struct{})
	go func() {
		defer close(ddnsDone)
		exitMgr.MonitorWireguardDDNS(ctx, ddnsInterval)
	}()
	server := api.NewServer(cfg, exitMgr, subMgr, cfgGen, ingress)
	listenErr := make(chan error, 1)
	go func() { listenErr <- server.Listen(addr) }()
	select {
	case err := <-listenErr:
		if err != nil {
			log.Fatalf("Server stopped: %v", err)
		}
	case <-ctx.Done():
		log.Println("Shutting down API and background schedulers")
		subMgr.Stop()
		restartScheduler.Stop()
		if err := server.Shutdown(); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}
	stopSignals()
	<-ddnsDone
	if sqlDB, err := db.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
