package store

import (
	"errors"
	"testing"
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
