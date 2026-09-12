package store

import (
	"testing"
	"time"
)

func TestAuditRoundTrip(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	when := time.Now().UTC()
	if err := database.SaveAudit(AuditEntry{ID: "audit-1", Timestamp: when, Actor: "admin", Action: "disk.plan", Outcome: "recorded", ResourceType: "disk", ResourceID: "wwn-a", Metadata: map[string]any{"safe": true}}); err != nil {
		t.Fatal(err)
	}
	entries, err := database.Audit(10)
	if err != nil || len(entries) != 1 || entries[0].Action != "disk.plan" || entries[0].Metadata["safe"] != true {
		t.Fatalf("unexpected audit entries: %#v err=%v", entries, err)
	}
}
