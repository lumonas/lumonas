package store

import (
	"database/sql"
	"path/filepath"
	"strings"
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

func TestStorageSnapshotMigrationAddsOriginAndPreservesRows(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "legacy-snapshot.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`DROP TABLE storage_snapshots; CREATE TABLE storage_snapshots (id TEXT PRIMARY KEY, kind TEXT NOT NULL, source TEXT NOT NULL, name TEXT NOT NULL, label TEXT, created_at TEXT NOT NULL, UNIQUE(kind, source, name)); INSERT INTO storage_snapshots(id,kind,source,name,label,created_at) VALUES('legacy','btrfs','/srv/pool','old','manual','2026-09-13T00:00:00Z')`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(filepath.Join(filepath.Dir(database.path), "legacy-snapshot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	records, err := reopened.StorageSnapshots("/srv/pool", 10)
	if err != nil || len(records) != 1 || records[0].Origin != "manual" {
		t.Fatalf("legacy snapshot migration failed: %#v err=%v", records, err)
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

func TestOpenMigratesLegacyJobObservabilitySchema(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-jobs.db")
	legacy, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE jobs (
id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL, resource_id TEXT,
state TEXT NOT NULL, progress REAL, stage TEXT, created_at TEXT NOT NULL,
started_at TEXT, finished_at TEXT, error TEXT
); INSERT INTO jobs(id,type,title,state,created_at) VALUES('legacy-job','smart.short','legacy SMART','queued',?)`, time.Now().UTC().Format(timeFormat)); err != nil {
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
	job, err := database.Job("legacy-job")
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != "legacy-job" || job.Type != "smart.short" || job.Generation == 0 {
		t.Fatalf("legacy job migration lost state or generation: %#v", job)
	}
	if _, err := database.db.Exec(`SELECT operation_id,plan_hash,actor,generation FROM jobs WHERE id='legacy-job'`); err != nil {
		t.Fatalf("job observability columns were not added: %v", err)
	}
}

func TestOpenMigratesLegacyRuntimeSchemaAsOneUpgrade(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-runtime.db")
	legacy, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Format(timeFormat)
	_, err = legacy.Exec(`
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE events (id TEXT PRIMARY KEY, type TEXT NOT NULL, timestamp TEXT NOT NULL, severity TEXT NOT NULL, resource_type TEXT, resource_id TEXT, data_json TEXT NOT NULL);
CREATE TABLE jobs (id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL, resource_id TEXT, state TEXT NOT NULL, progress REAL, stage TEXT, created_at TEXT NOT NULL, started_at TEXT, finished_at TEXT, error TEXT);
CREATE TABLE audit_log (id TEXT PRIMARY KEY, timestamp TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT, resource_id TEXT, metadata_json TEXT NOT NULL);
CREATE TABLE network_bindings (service TEXT PRIMARY KEY, config_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE network_firewall (id INTEGER PRIMARY KEY CHECK(id=1), config_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE network_connections (id TEXT PRIMARY KEY, uuid TEXT NOT NULL DEFAULT '', name TEXT NOT NULL, interface TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, config_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
INSERT INTO users(id,username,password_hash,created_at) VALUES('legacy-user','admin','legacy-hash',?);
INSERT INTO events(id,type,timestamp,severity,data_json) VALUES('legacy-event','legacy.test',?,'info','{}');
INSERT INTO jobs(id,type,title,state,created_at) VALUES('legacy-job','legacy.test','Legacy job','completed',?);
INSERT INTO audit_log(id,timestamp,actor,action,outcome,metadata_json) VALUES('legacy-audit',?,'admin','legacy.upgrade','recorded','{}');
INSERT INTO network_bindings(service,config_json,updated_at) VALUES('smb','{"service":"smb"}',?);
INSERT INTO network_firewall(id,config_json,updated_at) VALUES(1,'{"default":"deny","services":{}}',?);
INSERT INTO network_connections(id,uuid,name,interface,enabled,config_json,created_at,updated_at) VALUES('lan','legacy-uuid','LAN','eth0',1,'{"id":"lan","name":"LAN","interface":"eth0","enabled":true,"type":"ethernet","ipv4":{"method":"auto"},"ipv6":{"method":"disabled"}}',?,?);`,
		created, created, created, created, created, created, created, created)
	if err != nil {
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

	principals, err := database.ListPrincipals("")
	if err != nil || len(principals) != 1 || principals[0].Name != "admin" {
		t.Fatalf("legacy users were not promoted to principals: %#v err=%v", principals, err)
	}
	jobs, err := database.Jobs()
	if err != nil || len(jobs) != 1 || jobs[0].CorrelationID != "" {
		t.Fatalf("legacy jobs were not readable after migration: %#v err=%v", jobs, err)
	}
	var actorColumn int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('jobs') WHERE name='actor'`).Scan(&actorColumn); err != nil || actorColumn != 1 {
		t.Fatalf("legacy jobs did not receive actor column: count=%d err=%v", actorColumn, err)
	}
	audits, err := database.Audit(10)
	if err != nil || len(audits) != 1 || audits[0].ID != "legacy-audit" {
		t.Fatalf("legacy audit rows were not readable after migration: %#v err=%v", audits, err)
	}
	bindings, err := database.ListNetworkBindings()
	if err != nil || len(bindings) != 1 || bindings[0].Service != "smb" {
		t.Fatalf("legacy network bindings were not copied: %#v err=%v", bindings, err)
	}
	policy, err := database.NetworkFirewallPolicy()
	if err != nil || policy.Default != "deny" {
		t.Fatalf("legacy firewall policy was not copied: %#v err=%v", policy, err)
	}
	connections, err := database.ListNetworkConnections()
	if err != nil || len(connections) != 1 || connections[0].ID != "lan" {
		t.Fatalf("legacy network connection was not upgraded: %#v err=%v", connections, err)
	}
	var columns string
	if err := database.db.QueryRow(`SELECT group_concat(name, ',') FROM pragma_table_info('jobs')`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(columns, "correlation_id") {
		t.Fatalf("jobs migration did not add correlation_id: %s", columns)
	}
}
