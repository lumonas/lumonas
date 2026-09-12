package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestMigrateCreatesAndRecordsSchema(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "lumonas.db")
	if err := migrate(databasePath); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var migrationCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount < 1 {
		t.Fatalf("expected recorded migrations, got %d", migrationCount)
	}
	var generation string
	if err := database.QueryRow(`SELECT value FROM meta WHERE key = 'config_generation'`).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation != "1" {
		t.Fatalf("unexpected initial config generation %q", generation)
	}
}

func TestEnvOrUsesNonEmptyEnvironmentValue(t *testing.T) {
	t.Setenv("LUMONAS_MIGRATE_TEST", "custom.db")
	if got := envOr("LUMONAS_MIGRATE_TEST", "fallback.db"); got != "custom.db" {
		t.Fatalf("envOr returned %q", got)
	}
	if got := envOr("LUMONAS_MIGRATE_MISSING", "fallback.db"); got != "fallback.db" {
		t.Fatalf("envOr fallback returned %q", got)
	}
}
