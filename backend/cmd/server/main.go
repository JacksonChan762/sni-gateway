package main

import (
	"log"
	"os"
	"sni-gateway/internal/api"
	"sni-gateway/internal/config"
	"sni-gateway/internal/db"
	"sni-gateway/internal/docker"
	"sni-gateway/internal/services"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

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

	var reloader services.NginxReloader
	if dockerClient != nil {
		reloader = dockerClient
	}
	cfgGen := services.NewConfigGenerator(cfg.NginxConfigDir, cfg.NginxPID, reloader)
	subMgr := services.NewSubscriptionManager(cfgGen)

	// Write initial sni_map.conf so nginx can start successfully
	if err := cfgGen.WriteInitial(); err != nil {
		log.Printf("Warning: could not write initial nginx config: %v", err)
	} else {
		log.Println("Initial nginx config written.")
	}

	subMgr.Start()
	defer subMgr.Stop()

	server := api.NewServer(cfg, exitMgr, subMgr, cfgGen)

	addr := ":" + cfg.Port
	log.Printf("SNI Gateway backend starting on %s", addr)

	if err := server.Listen(addr); err != nil {
		log.Fatal(err)
	}

	_ = os.Getenv // suppress unused import
}
