package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestCapacitySnapshotsAreBoundedToOnePerResourcePerDay(t *testing.T) {
	db, err := Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	if err := db.SaveCapacitySnapshot(model.CapacitySnapshot{ResourceID: "pool", CapturedAt: first, TotalBytes: 100, UsedBytes: 10}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCapacitySnapshot(model.CapacitySnapshot{ResourceID: "pool", CapturedAt: first.Add(10 * time.Hour), TotalBytes: 100, UsedBytes: 20}); err != nil {
		t.Fatal(err)
	}
	items, err := db.CapacitySnapshots("pool", first.Add(-time.Hour), 10)
	if err != nil || len(items) != 1 || items[0].UsedBytes != 20 {
		t.Fatalf("unexpected snapshots: %#v err=%v", items, err)
	}
}
