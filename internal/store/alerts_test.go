package store

import (
	"testing"
	"time"
)

func TestPendingAlertWindowPersistsAndClears(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	started := time.Now().UTC().Add(-time.Minute)
	want := PendingAlert{RuleID: "rule-temp", ResourceID: "disk-1", Severity: "warning", Title: "Hot disk", Description: "too warm", StartedAt: started}
	if err := database.SavePendingAlert(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := database.PendingAlert(want.RuleID, want.ResourceID)
	if err != nil || !found {
		t.Fatalf("pending alert was not persisted: %#v found=%v err=%v", got, found, err)
	}
	if got.Title != want.Title || got.Description != want.Description || got.StartedAt.Unix() != want.StartedAt.Unix() {
		t.Fatalf("unexpected pending alert: %#v", got)
	}
	if err := database.ClearPendingAlert(want.RuleID, want.ResourceID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := database.PendingAlert(want.RuleID, want.ResourceID); err != nil || found {
		t.Fatalf("pending alert was not cleared: found=%v err=%v", found, err)
	}
}
