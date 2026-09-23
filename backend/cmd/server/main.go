package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sni-gateway/internal/api"
	"sni-gateway/internal/catalog"
	"sni-gateway/internal/config"
	"sni-gateway/internal/db"
	"sni-gateway/internal/docker"
	"sni-gateway/internal/gateway"
	"sni-gateway/internal/services"
	"syscall"
	"time"

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

	server := api.NewServer(cfg, exitMgr, subMgr, cfgGen)

	addr := ":" + cfg.Port
	log.Printf("SNI Gateway backend starting on %s", addr)

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	catalog.StartUpdater(ctx, cfg.ProviderCatalogPath, updateInterval)
	ingress := &gateway.Server{Router: cfgGen, HTTPAddr: ":80", TLSAddr: ":443"}
	if err := ingress.Start(ctx); err != nil {
		log.Fatalf("Start SNI ingress: %v", err)
	}
	defer ingress.Close()
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
	if sqlDB, err := db.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
