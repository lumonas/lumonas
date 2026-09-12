package store

import (
	"path/filepath"
	"testing"
)

func TestStoreReopenPreservesStateAcrossMigrations(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "lumonas.db")
	database, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetMeta("upgrade-canary", "preserved"); err != nil {
		database.Close()
		t.Fatal(err)
	}
	generation, err := database.BeginGeneration("upgrade-test")
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.CommitGeneration(generation); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if value, ok := reopened.Meta("upgrade-canary"); !ok || value != "preserved" {
		t.Fatalf("migration reopen lost metadata: %q %v", value, ok)
	}
	if reopened.CurrentGeneration() != generation {
		t.Fatalf("migration reopen lost generation: got %d want %d", reopened.CurrentGeneration(), generation)
	}
	var migrationCount int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount < 1 {
		t.Fatalf("expected recorded schema migrations, got %d", migrationCount)
	}
}
