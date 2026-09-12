package storage

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func testDisk() model.Disk {
	return model.Disk{ID: "wwn:test", CurrentPath: "/dev/sdb", Model: "Test Disk", Serial: "SER-1", WWN: "test", SizeBytes: 1000, GPTDiskGUID: "GPT-1", PartitionUUID: "PART-1", Role: "unknown", Health: model.Healthy}
}

func TestPlanHashAndIdentityValidation(t *testing.T) {
	now := time.Now().UTC()
	plan, err := NewPlan("op-1", ActionFormat, testDisk(), 7, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanHash != Hash(plan) {
		t.Fatal("plan hash is not stable")
	}
	if err := Validate(plan, testDisk(), now.Add(time.Minute), 7); err != nil {
		t.Fatal(err)
	}
	changed := testDisk()
	changed.Serial = "SER-2"
	if err := Validate(plan, changed, now.Add(time.Minute), 7); err == nil {
		t.Fatal("serial mismatch should fail closed")
	}
	changed = testDisk()
	changed.GPTDiskGUID = "GPT-2"
	if err := Validate(plan, changed, now.Add(time.Minute), 7); err == nil {
		t.Fatal("GPT disk GUID mismatch should fail closed")
	}
	changed = testDisk()
	changed.PartitionUUID = "PART-2"
	if err := Validate(plan, changed, now.Add(time.Minute), 7); err == nil {
		t.Fatal("partition UUID mismatch should fail closed")
	}
}

func TestPlanRejectsExpiryMountAndGenerationRaces(t *testing.T) {
	now := time.Now().UTC()
	plan, err := NewPlan("op-2", ActionErase, testDisk(), 7, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(plan, testDisk(), now.Add(16*time.Minute), 7); err == nil {
		t.Fatal("expired plan should fail")
	}
	if err := Validate(plan, testDisk(), now.Add(time.Minute), 8); err == nil {
		t.Fatal("generation race should fail")
	}
	mounted := testDisk()
	mounted.Mounted = true
	if err := Validate(plan, mounted, now.Add(time.Minute), 7); err == nil {
		t.Fatal("mounted destructive target should fail")
	}
}

func TestCreatePlanValidatesRequestedState(t *testing.T) {
	now := time.Now().UTC()
	plan, err := NewPlan("op-3", ActionCreate, testDisk(), 7, now)
	if err != nil {
		t.Fatal(err)
	}
	plan.RequestedState = map[string]any{"filesystem": "ext4", "mountPath": DiskBranchPath(plan.Target.DiskID), "label": "media"}
	plan.PlanHash = Hash(plan)
	if err := Validate(plan, testDisk(), now.Add(time.Minute), 7); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestedState(ActionCreate, plan.Target.DiskID, map[string]any{"filesystem": "btrfs", "mountPath": DiskBranchPath(plan.Target.DiskID)}); err == nil {
		t.Fatal("unsupported filesystem should fail")
	}
	if err := ValidateRequestedState(ActionCreate, plan.Target.DiskID, map[string]any{"filesystem": "ext4", "mountPath": "/mnt/other"}); err == nil {
		t.Fatal("non-canonical mount path should fail")
	}
	if err := ValidateRequestedState(ActionCreate, plan.Target.DiskID, map[string]any{"filesystem": "ext4", "mountPath": DiskBranchPath(plan.Target.DiskID), "label": "-bad label"}); err == nil {
		t.Fatal("invalid label should fail")
	}
	if err := ValidateRequestedState(ActionCreate, plan.Target.DiskID, map[string]any{"filesystem": "ext4", "mountPath": DiskBranchPath(plan.Target.DiskID), "label": "thirteenchars"}); err == nil {
		t.Fatal("overlong label should fail")
	}
	if err := ValidateRequestedState(ActionMount, "x", map[string]any{"anything": true}); err != nil {
		t.Fatalf("non-create actions skip requested-state validation: %v", err)
	}
	mounted := testDisk()
	mounted.Mounted = true
	if err := Validate(plan, mounted, now.Add(time.Minute), 7); err == nil {
		t.Fatal("create on a mounted target should fail")
	}
}
