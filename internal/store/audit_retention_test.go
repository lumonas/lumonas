package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestPruneAuditKeepsNewestWindow(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for index := 0; index < 105; index++ {
		if err := database.SaveAudit(AuditEntry{ID: "audit-" + time.Now().UTC().Format("150405.000000000") + string(rune(index)), Actor: "system", Action: "test", Outcome: "recorded"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.PruneAudit(100); err != nil {
		t.Fatal(err)
	}
	items, err := database.Audit(500)
	if err != nil || len(items) != 100 {
		t.Fatalf("expected 100 audit entries, got %d err=%v", len(items), err)
	}
}

func TestDiskInventoryRoundTrip(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveDiskInventory([]model.Disk{{ID: "wwn:a", Model: "Test", SizeBytes: 42, Role: "data", LastSeen: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	disks, err := database.KnownDisks()
	if err != nil || len(disks) != 1 || disks[0].ID != "wwn:a" || disks[0].Role != "data" {
		t.Fatalf("unexpected disk inventory: %#v err=%v", disks, err)
	}
}
