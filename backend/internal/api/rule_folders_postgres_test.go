package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"sni-gateway/internal/config"
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"
)

func TestRuleFolderMigrationWithPostgreSQL(t *testing.T) {
	initLegacyRulesPostgres(t)
	publisher, err := services.NewRoutePublisher(filepath.Join(t.TempDir(), "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "rule-folder-test"}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	}).SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, nil, nil, publisher, nil)
	request := func(method, path, body string, status int, dest any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := server.app.Test(req, 5000)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, res.StatusCode, status, data)
		}
		if dest != nil {
			if err := json.Unmarshal(data, dest); err != nil {
				t.Fatalf("decode %s %s: %v", method, path, err)
			}
		}
	}
	readRule := func(folder string) models.Rule {
		t.Helper()
		var rules []models.Rule
		request(http.MethodGet, "/api/rules?folder_id="+folder, "", http.StatusOK, &rules)
		if len(rules) != 1 {
			t.Fatalf("folder %q contains %d rules, want 1", folder, len(rules))
		}
		rule := rules[0]
		rule.CreatedAt = rule.CreatedAt.UTC()
		rule.UpdatedAt = rule.UpdatedAt.UTC()
		return rule
	}

	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	legacy := models.Rule{
		ID: 1, Pattern: "legacy.example", Type: "suffix", Priority: 75,
		Source: "manual", Enabled: true, CreatedAt: created, UpdatedAt: created.Add(48 * time.Hour),
	}
	if got := readRule("unfiled"); !reflect.DeepEqual(got, legacy) {
		t.Fatalf("migration changed the legacy rule: got %+v, want %+v", got, legacy)
	}
	baseline, err := publisher.Apply()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(baseline, legacy.Pattern) {
		t.Fatalf("initial publish omitted the legacy rule: %s", baseline)
	}
	checkPublished := func(operation string) {
		t.Helper()
		snapshot, err := publisher.Preview()
		if err != nil {
			t.Fatal(err)
		}
		if snapshot != baseline {
			t.Fatalf("%s changed the route snapshot:\nbefore %s\nafter %s", operation, baseline, snapshot)
		}
		publisher.InvalidatePending()
		pending, err := publisher.HasPendingChanges()
		if err != nil || pending {
			t.Fatalf("%s left pending routing changes: pending=%v err=%v", operation, pending, err)
		}
	}

	var folder models.RuleFolder
	request(http.MethodPost, "/api/rule-folders", `{"name":"Work"}`, http.StatusCreated, &folder)
	request(http.MethodPut, fmt.Sprintf("/api/rules/%d", legacy.ID), fmt.Sprintf(
		`{"pattern":%q,"type":%q,"priority":%d,"exit_id":null,"enabled":true,"folder_id":%d}`,
		legacy.Pattern, legacy.Type, legacy.Priority, folder.ID), http.StatusOK, nil)
	wantMoved := legacy
	wantMoved.FolderID = &folder.ID
	if got := readRule(fmt.Sprint(folder.ID)); !reflect.DeepEqual(got, wantMoved) {
		t.Fatalf("move changed the rule: got %+v, want %+v", got, wantMoved)
	}
	checkPublished("moving the rule")

	request(http.MethodDelete, fmt.Sprintf("/api/rule-folders/%d", folder.ID), "", http.StatusOK, nil)
	if got := readRule("unfiled"); !reflect.DeepEqual(got, legacy) {
		t.Fatalf("folder deletion changed the rule: got %+v, want %+v", got, legacy)
	}
	checkPublished("deleting the folder")
}

func initLegacyRulesPostgres(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("SNI_GATEWAY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set SNI_GATEWAY_TEST_POSTGRES_DSN to a PostgreSQL URL to test migrations")
	}
	scopedDSN, err := url.Parse(dsn)
	if err != nil || (scopedDSN.Scheme != "postgres" && scopedDSN.Scheme != "postgresql") {
		t.Fatal("SNI_GATEWAY_TEST_POSTGRES_DSN must use a postgres:// or postgresql:// URL")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	adminPool, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminPool.Close() })

	// Restrict migration and API connections to a fresh schema for this test.
	schema := "sni_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	if err := admin.Exec(`CREATE TABLE ` + schema + `.rules (
		id bigserial PRIMARY KEY, pattern text NOT NULL, type text NOT NULL,
		exit_id bigint, priority bigint DEFAULT 100, source text DEFAULT 'manual',
		sub_id bigint, enabled boolean DEFAULT true, created_at timestamptz, updated_at timestamptz
	)`).Error; err != nil {
		t.Fatalf("create legacy rules table: %v", err)
	}
	if err := admin.Exec(`INSERT INTO ` + schema + `.rules (pattern, type, priority, created_at, updated_at)
		VALUES ('legacy.example', 'suffix', 75, '2025-01-01T00:00:00Z', '2025-01-03T00:00:00Z')`).Error; err != nil {
		t.Fatalf("seed legacy rule: %v", err)
	}
	params := scopedDSN.Query()
	params.Set("search_path", schema)
	scopedDSN.RawQuery = params.Encode()
	previous := db.DB
	t.Cleanup(func() { db.DB = previous })
	migrateErr := db.Init(scopedDSN.String())
	if db.DB != nil {
		if pool, err := db.DB.DB(); err == nil {
			t.Cleanup(func() { _ = pool.Close() })
		}
	}
	if migrateErr != nil {
		t.Fatalf("migrate legacy schema: %v", migrateErr)
	}
}
