package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port              string
	DatabaseURL       string
	JWTSecret         string
	AdminUsername     string
	AdminPassword     string
	RouteSnapshotPath string
	DockerSocket      string
	ExitRestartCron   string
}

func Load() *Config {
	return &Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://sni:sni@postgres:5432/sni_gateway?sslmode=disable"),
		JWTSecret:         getEnv("JWT_SECRET", "change-me-in-production"),
		AdminUsername:     getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:     getEnv("ADMIN_PASSWORD", "admin"),
		RouteSnapshotPath: getEnv("ROUTE_SNAPSHOT_PATH", "/var/lib/sni-gateway/routes.json"),
		DockerSocket:      getEnv("DOCKER_SOCKET", "/var/run/docker.sock"),
		ExitRestartCron:   getEnv("EXIT_RESTART_CRON", getEnv("NIGHTLY_RESTART_EXITS_CRON", "@every 12h")),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
