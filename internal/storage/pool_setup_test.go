package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func poolSetupTestDisk(id, filesystem string, mounted bool) model.Disk {
	return model.Disk{
		ID:             id,
		CurrentPath:    "/dev/" + id,
		WWN:            "wwn-" + id,
		Serial:         "serial-" + id,
		Model:          "TestDisk",
		SizeBytes:      100,
		GPTDiskGUID:    "gpt-" + id,
		PartitionUUID:  "partition-" + id,
		FilesystemUUID: "filesystem-" + id,
		Filesystem:     filesystem,
		Mounted:        mounted,
		Health:         model.Healthy,
	}
}

func TestPoolSetupPlanIncludesMountsAndImmutableIdentities(t *testing.T) {
	now := time.Now().UTC()
	data := poolSetupTestDisk("data", "ext4", false)
	parity := poolSetupTestDisk("parity", "", false)
	parity.Role = "parity"
	plan, err := NewPoolSetupPlan("setup-1", "media", []model.Disk{data, parity}, []string{"data"}, "parity", "ext4", false, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.FormatDiskIDs) != 1 || plan.FormatDiskIDs[0] != "parity" {
		t.Fatalf("expected blank parity to be formatted, got %#v", plan.FormatDiskIDs)
	}
	if len(plan.MountDiskIDs) != 1 || plan.MountDiskIDs[0] != "data" {
		t.Fatalf("expected existing data filesystem to be mounted, got %#v", plan.MountDiskIDs)
	}
	if len(plan.ExpectedDisks) != 2 || plan.ExpectedDisks[0].Serial != "serial-data" {
		t.Fatalf("expected stable identity snapshots, got %#v", plan.ExpectedDisks)
	}
	if err := ValidatePoolSetupPlan(plan, []model.Disk{parity, data}, now.Add(time.Minute), 7); err != nil {
		t.Fatalf("device reorder should remain safe: %v", err)
	}
	if plan.PlanHash != HashPoolSetupPlan(plan) {
		t.Fatal("pool setup plan hash is not stable")
	}
}

func TestPoolSetupPlanFailsClosedOnReplacementGenerationAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	disk := poolSetupTestDisk("data", "", false)
	plan, err := NewPoolSetupPlan("setup-2", "media", []model.Disk{disk}, []string{"data"}, "", "ext4", false, 9, now)
	if err != nil {
		t.Fatal(err)
	}
	replaced := disk
	replaced.Serial = "replacement"
	if err := ValidatePoolSetupPlan(plan, []model.Disk{replaced}, now.Add(time.Minute), 9); err == nil || !strings.Contains(err.Error(), "serial mismatch") {
		t.Fatalf("expected serial replacement rejection, got %v", err)
	}
	if err := ValidatePoolSetupPlan(plan, []model.Disk{disk}, now.Add(time.Minute), 10); err == nil {
		t.Fatal("generation change should fail closed")
	}
	if err := ValidatePoolSetupPlan(plan, []model.Disk{disk}, now.Add(16*time.Minute), 9); err == nil {
		t.Fatal("expired plan should fail closed")
	}
}
