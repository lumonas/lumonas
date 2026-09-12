package store

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestOpenMigratesLegacyObservabilityColumns(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE events (id TEXT PRIMARY KEY, type TEXT NOT NULL, timestamp TEXT NOT NULL, severity TEXT NOT NULL, resource_type TEXT, resource_id TEXT, data_json TEXT NOT NULL);
CREATE TABLE jobs (id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL, resource_id TEXT, state TEXT NOT NULL, progress REAL, stage TEXT, created_at TEXT NOT NULL, started_at TEXT, finished_at TEXT, error TEXT);
CREATE TABLE audit_log (id TEXT PRIMARY KEY, timestamp TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT, resource_id TEXT, metadata_json TEXT NOT NULL);`)
	if err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveEvent(model.Event{ID: "legacy-event", Type: "legacy", Timestamp: time.Now().UTC(), Severity: "info", OperationID: "op-legacy", Data: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveAudit(AuditEntry{ID: "legacy-audit", Actor: "system", Action: "legacy", Outcome: "recorded", CorrelationID: "corr-legacy"}); err != nil {
		t.Fatal(err)
	}
	events, err := database.Events(10)
	if err != nil || len(events) != 1 || events[0].OperationID != "op-legacy" {
		t.Fatalf("legacy event columns were not migrated: %#v err=%v", events, err)
	}
	audits, err := database.Audit(10)
	if err != nil || len(audits) != 1 || audits[0].CorrelationID != "corr-legacy" {
		t.Fatalf("legacy audit columns were not migrated: %#v err=%v", audits, err)
	}
}

func TestEventFieldsPersistOutsideMetadata(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveEvent(model.Event{
		ID: "event-fields", Type: "storage.operation.completed", Timestamp: time.Now().UTC(), Severity: "info",
		CorrelationID: "corr-1", OperationID: "op-1", PlanHash: "plan-1", Actor: "admin", Generation: 9,
		Data: map[string]any{"detail": "safe"},
	}); err != nil {
		t.Fatal(err)
	}
	values, err := database.Events(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].CorrelationID != "corr-1" || values[0].OperationID != "op-1" || values[0].PlanHash != "plan-1" || values[0].Actor != "admin" || values[0].Generation != 9 {
		t.Fatalf("event fields were not persisted: %#v", values)
	}
}

func TestAuditFieldsAreDerivedAndPersisted(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveAudit(AuditEntry{
		ID: "audit-fields", Actor: "admin", Action: "storage.plan", Outcome: "recorded",
		Metadata: map[string]any{"correlationId": "corr-2", "operationId": "op-2", "planHash": "plan-2", "generation": 10},
	}); err != nil {
		t.Fatal(err)
	}
	values, err := database.Audit(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].CorrelationID != "corr-2" || values[0].OperationID != "op-2" || values[0].PlanHash != "plan-2" || values[0].Generation != 10 {
		t.Fatalf("audit fields were not persisted: %#v", values)
	}
}
