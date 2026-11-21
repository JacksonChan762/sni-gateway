package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sni-gateway/internal/config"
	"sni-gateway/internal/db"
	"sni-gateway/internal/gateway"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDashboardStatsReportsConfiguredTLSLimit(t *testing.T) {
	// The dashboard's existing database counts do not need a live PostgreSQL server.
	database, err := gorm.Open(postgres.Open("host=127.0.0.1 dbname=sni_gateway"), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	previousDB := db.DB
	t.Cleanup(func() { db.DB = previousDB })
	db.DB = database

	cfg := &config.Config{JWTSecret: "dashboard-test-secret"}
	ingress := &gateway.Server{MaxConnections: 4096}
	server := NewServer(cfg, nil, nil, nil, ingress)
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"exp": time.Now().Add(time.Minute).Unix()})
	signed, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+signed)
	response, err := server.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("dashboard stats returned status %d", response.StatusCode)
	}
	var stats struct {
		TLS gateway.TLSStats `json:"tls"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if want := (gateway.TLSStats{Limit: 4096}); stats.TLS != want {
		t.Fatalf("TLS stats = %+v, want %+v", stats.TLS, want)
	}
}
