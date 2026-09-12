package storage

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func testDisk() model.Disk {
	return model.Disk{ID: "wwn:test", CurrentPath: "/dev/sdb", Model: "Test Disk", Serial: "SER-1", WWN: "test", SizeBytes: 1000, Role: "unknown", Health: model.Healthy}
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
