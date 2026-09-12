package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	_ "github.com/mattn/go-sqlite3"
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
	if err := database.SaveEvent(model.Event{ID: "upgrade-event", Type: "upgrade.test", Timestamp: time.Now().UTC(), Severity: "info", Data: map[string]any{}}); err != nil {
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
	events, err := reopened.Events(10)
	if err != nil || len(events) != 1 || events[0].SchemaVersion != 1 {
		t.Fatalf("event schema version was not preserved: %#v err=%v", events, err)
	}
	var migrationCount int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount < 1 {
		t.Fatalf("expected recorded schema migrations, got %d", migrationCount)
	}
}

func TestOpenMigratesLegacyEventSchema(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE events (
id TEXT PRIMARY KEY, type TEXT NOT NULL, timestamp TEXT NOT NULL,
severity TEXT NOT NULL, resource_type TEXT, resource_id TEXT, data_json TEXT NOT NULL
)`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO events(id,type,timestamp,severity,data_json) VALUES('legacy-event','legacy.test',?,'info','{}')`, time.Now().UTC().Format(timeFormat)); err != nil {
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
	items, err := database.Events(10)
	if err != nil || len(items) != 1 || items[0].ID != "legacy-event" || items[0].SchemaVersion != 1 {
		t.Fatalf("legacy event migration failed: %#v err=%v", items, err)
	}
}
