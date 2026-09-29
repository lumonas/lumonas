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

func TestAuditPageFiltersAndStableCursor(t *testing.T) {
	database, err := Open(t.TempDir() + "/audit-page.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	when := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	for _, entry := range []AuditEntry{
		{ID: "a", Timestamp: when, Actor: "alice", Action: "share.update", Outcome: "committed", ResourceType: "share", ResourceID: "media"},
		{ID: "b", Timestamp: when, Actor: "bob", Action: "share.delete", Outcome: "committed", ResourceType: "share", ResourceID: "archive"},
		{ID: "c", Timestamp: when.Add(time.Minute), Actor: "alice", Action: "disk.plan", Outcome: "recorded", ResourceType: "disk", ResourceID: "disk-1"},
	} {
		if err := database.SaveAudit(entry); err != nil {
			t.Fatal(err)
		}
	}
	filters := AuditFilters{Actor: "alice"}
	first, err := database.AuditPage(filters, 1, "")
	if err != nil || len(first.Entries) != 1 || !first.HasMore || first.Entries[0].ID != "c" {
		t.Fatalf("unexpected first audit page: %#v err=%v", first, err)
	}
	second, err := database.AuditPage(filters, 1, first.NextCursor)
	if err != nil || len(second.Entries) != 1 || second.HasMore || second.Entries[0].ID != "a" {
		t.Fatalf("unexpected second audit page: %#v err=%v", second, err)
	}
	if _, err := database.AuditPage(filters, 1, "invalid-cursor"); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
	search, err := database.AuditPage(AuditFilters{Query: "%"}, 10, "")
	if err != nil || len(search.Entries) != 0 {
		t.Fatalf("audit search did not escape SQL wildcards: %#v err=%v", search, err)
	}
}
