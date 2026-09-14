package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func growPool() model.Pool {
	return model.Pool{
		ID: "pool-1", Name: "media", MountPath: "/srv/pools/media", Type: "mergerfs",
		Members: []model.PoolMember{
			{Enabled: true, BranchPath: "/srv/disks/serial_A"},
			{Enabled: true, BranchPath: "/srv/disks/serial_B"},
		},
	}
}

func growDisk(id, filesystem, role string) model.Disk {
	return model.Disk{ID: id, CurrentPath: "/dev/sdx", Role: role, Filesystem: filesystem, Health: model.Healthy, LastSeen: time.Now().UTC()}
}

func TestPoolMembershipPlanFormatsOnlyBlankDisks(t *testing.T) {
	disks := []model.Disk{growDisk("serial:A", "ext4", "data"), growDisk("serial:B", "xfs", "data"), growDisk("serial:C", "", "data")}
	plan, err := NewPoolMembershipPlan("op-1", growPool(), disks, []string{"serial:C"}, "", false, 3, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.FormatDiskIDs) != 1 || plan.FormatDiskIDs[0] != "serial:C" {
		t.Fatalf("unexpected format list: %#v", plan.FormatDiskIDs)
	}
	if len(plan.NewBranches) != 3 {
		t.Fatalf("new branches = %#v", plan.NewBranches)
	}
	if plan.NewBranches[2] != "/srv/disks/serial_C" {
		t.Fatalf("new member branch wrong: %#v", plan.NewBranches)
	}

	// A pre-formatted new disk joins without any destructive step.
	disks = append(disks, growDisk("serial:D", "ext4", "data"))
	clean, err := NewPoolMembershipPlan("op-2", growPool(), disks, []string{"serial:D"}, "", false, 3, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(clean.FormatDiskIDs) != 0 {
		t.Fatalf("formatted disk queued for formatting: %#v", clean.FormatDiskIDs)
	}
}

func TestPoolMembershipPlanRejectsBadGrowRequests(t *testing.T) {
	disks := []model.Disk{growDisk("serial:A", "ext4", "data"), growDisk("serial:B", "ext4", "data"), growDisk("serial:P", "", "parity")}
	cases := []struct {
		name string
		pool model.Pool
		add  []string
		want string
	}{
		{"unknown disk", growPool(), []string{"serial:ZZ"}, "not currently discovered"},
		{"parity disk", growPool(), []string{"serial:P"}, "parity disk"},
		{"nothing to add", growPool(), nil, "at least one disk"},
		{"non-canonical mount path", model.Pool{Name: "media", MountPath: "/mnt/media", Members: growPool().Members}, []string{"serial:A"}, "mount path is invalid"},
	}
	for _, testCase := range cases {
		_, err := NewPoolMembershipPlan("op", testCase.pool, disks, testCase.add, "", false, 1, time.Now())
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Fatalf("%s: expected %q error, got %v", testCase.name, testCase.want, err)
		}
	}
}

func TestPoolMembershipValidationRejectsDrift(t *testing.T) {
	disks := []model.Disk{growDisk("serial:A", "ext4", "data"), growDisk("serial:B", "ext4", "data"), growDisk("serial:C", "ext4", "data")}
	plan, err := NewPoolMembershipPlan("op-1", growPool(), disks, []string{"serial:C"}, "", false, 3, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	grownPool := growPool()
	grownPool.Members = append(grownPool.Members, model.PoolMember{Enabled: true, BranchPath: "/srv/disks/serial_C"})

	if err := ValidatePoolMembershipPlan(plan, []model.Pool{growPool()}, disks, time.Now().Add(time.Minute), 3); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	// Pool unmounted after planning.
	if err := ValidatePoolMembershipPlan(plan, nil, disks, time.Now().Add(time.Minute), 3); err == nil || !strings.Contains(err.Error(), "no longer mounted") {
		t.Fatalf("unmounted pool accepted: %v", err)
	}
	// Someone else already added the branch: drift must fail.
	if err := ValidatePoolMembershipPlan(plan, []model.Pool{grownPool}, disks, time.Now().Add(time.Minute), 3); err == nil || !strings.Contains(err.Error(), "already mounted") {
		t.Fatalf("concurrent growth accepted: %v", err)
	}
	// New disk vanished.
	if err := ValidatePoolMembershipPlan(plan, []model.Pool{growPool()}, disks[:2], time.Now().Add(time.Minute), 3); err == nil || !strings.Contains(err.Error(), "no longer present") {
		t.Fatalf("missing disk accepted: %v", err)
	}
	// Generation moved.
	if err := ValidatePoolMembershipPlan(plan, []model.Pool{grownPool}, disks, time.Now().Add(time.Minute), 4); err == nil || !strings.Contains(err.Error(), "generation") {
		t.Fatalf("stale generation accepted: %v", err)
	}
	// Tampered hash.
	plan.AddDiskIDs = []string{"serial:B"}
	if err := ValidatePoolMembershipPlan(plan, []model.Pool{grownPool}, disks, time.Now().Add(time.Minute), 3); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("tampered plan accepted: %v", err)
	}
}

func TestExtendSnapraidSlotsAssignsNextFreeNames(t *testing.T) {
	slots := []DataSlot{{Name: "d1", DiskID: "serial_A"}, {Name: "d2", DiskID: "serial_B"}}
	updated, err := ExtendSnapraidSlots(slots, []string{"serial:C"})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 3 || updated[2].Name != "d3" || updated[2].DiskID != "serial_C" {
		t.Fatalf("unexpected extension: %#v", updated)
	}
	// Original slots untouched.
	if updated[0].Name != "d1" || updated[1].Name != "d2" {
		t.Fatalf("existing slots changed: %#v", updated)
	}
	// Adding the same disk twice is rejected.
	if _, err := ExtendSnapraidSlots(updated, []string{"serial:C"}); err == nil {
		t.Fatal("duplicate disk accepted")
	}
	// Non-contiguous names pick max+1.
	sparse := []DataSlot{{Name: "d1", DiskID: "serial_A"}, {Name: "d5", DiskID: "serial_B"}}
	updated, err = ExtendSnapraidSlots(sparse, []string{"serial:C"})
	if err != nil {
		t.Fatal(err)
	}
	if updated[2].Name != "d6" {
		t.Fatalf("expected d6, got %q", updated[2].Name)
	}
}
