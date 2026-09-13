package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestLanHostsSchemaIsMigratedAtOpen(t *testing.T) {
	database := testStore(t)
	var version int
	if err := database.db.QueryRow(`SELECT version FROM schema_migrations WHERE version=12`).Scan(&version); err != nil || version != 12 {
		t.Fatalf("LAN host schema migration was not recorded: %d err=%v", version, err)
	}
	if _, err := database.db.Exec(`SELECT mac FROM lan_hosts LIMIT 1`); err != nil {
		t.Fatalf("LAN host table is not available after Open: %v", err)
	}
}

func TestLanHostMutationsRequireKnownTarget(t *testing.T) {
	database := testStore(t)
	if err := database.RenameLanHost("aa:bb:cc:dd:ee:ff", "eth0", "nas"); !errors.Is(err, ErrLanHostNotFound) {
		t.Fatalf("unknown rename should fail closed, got %v", err)
	}
	known, err := database.LanHostKnown("aa:bb:cc:dd:ee:ff", "eth0")
	if err != nil || known {
		t.Fatalf("unknown host should not be reported as known: %v %v", known, err)
	}
	if err := database.UpsertLanHosts([]LanHostRecord{{MAC: "aa:bb:cc:dd:ee:ff", Interface: "eth0", IP: "192.168.1.10"}}); err != nil {
		t.Fatal(err)
	}
	known, err = database.LanHostKnown("aa:bb:cc:dd:ee:ff", "eth0")
	if err != nil || !known {
		t.Fatalf("known host should be discoverable: %v %v", known, err)
	}
	if err := database.RenameLanHost("aa:bb:cc:dd:ee:ff", "eth0", "nas"); err != nil {
		t.Fatal(err)
	}
}

func TestOpenMigratesLegacyLanHostSchema(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-lan.db")
	legacy, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL); CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var version int
	if err := database.db.QueryRow(`SELECT version FROM schema_migrations WHERE version=12`).Scan(&version); err != nil || version != 12 {
		t.Fatalf("legacy LAN schema migration was not recorded: %d err=%v", version, err)
	}
	if _, err := database.db.Exec(`SELECT mac FROM lan_hosts LIMIT 1`); err != nil {
		t.Fatalf("legacy database did not receive LAN host table: %v", err)
	}
}
