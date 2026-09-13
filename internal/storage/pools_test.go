package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func poolDisk(id, path string, size uint64) model.Disk {
	return model.Disk{ID: id, CurrentPath: path, WWN: id + "-wwn", Serial: id + "-serial", Model: "TestDisk", SizeBytes: size, GPTDiskGUID: id + "-gpt", PartitionUUID: id + "-partition", Filesystem: "xfs", Health: model.Healthy}
}

func TestPoolPlanUsesStableIdentitiesAndCanonicalBranches(t *testing.T) {
	disks := []model.Disk{poolDisk("wwn:a", "/dev/sda", 100), poolDisk("wwn:b", "/dev/sdb", 200)}
	plan, err := NewPoolPlan("pool-1", "media", "/srv/pools/media", disks, 7, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePoolPlan(plan, disks, time.Now().UTC(), 7); err != nil {
		t.Fatal(err)
	}
	if plan.Members[0].BranchPath != "/srv/disks/wwn_a" {
		t.Fatalf("unexpected branch path %q", plan.Members[0].BranchPath)
	}
	command, err := MergerFSCommand(plan)
	if err != nil || !strings.Contains(strings.Join(command, " "), "/srv/pools/media") {
		t.Fatalf("unexpected mergerfs command %#v err=%v", command, err)
	}
}

func TestPoolPlanRejectsKernelPathIdentity(t *testing.T) {
	disk := poolDisk("path:/dev/sda", "/dev/sda", 100)
	if _, err := NewPoolPlan("pool-1", "media", "/srv/pools/media", []model.Disk{disk}, 7, time.Now().UTC()); err == nil {
		t.Fatal("pool plan accepted a kernel-path identity")
	}
}

func TestPoolPlanFailsClosedOnReorderedOrReplacedDisk(t *testing.T) {
	disks := []model.Disk{poolDisk("wwn:a", "/dev/sda", 100), poolDisk("wwn:b", "/dev/sdb", 200)}
	plan, err := NewPoolPlan("pool-1", "media", "/srv/pools/media", disks, 7, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePoolPlan(plan, []model.Disk{disks[1], disks[0]}, time.Now().UTC(), 7); err != nil {
		t.Fatalf("device-letter reorder should be safe: %v", err)
	}
	replaced := []model.Disk{disks[0], poolDisk("wwn:b", "/dev/sdc", 201)}
	if err := ValidatePoolPlan(plan, replaced, time.Now().UTC(), 7); err == nil || !strings.Contains(err.Error(), "capacity mismatch") {
		t.Fatalf("expected replacement rejection, got %v", err)
	}
	replaced = []model.Disk{disks[0], poolDisk("wwn:b", "/dev/sdc", 200)}
	replaced[1].GPTDiskGUID = "replacement-gpt"
	if err := ValidatePoolPlan(plan, replaced, time.Now().UTC(), 7); err == nil || !strings.Contains(err.Error(), "GPT disk GUID mismatch") {
		t.Fatalf("expected GPT replacement rejection, got %v", err)
	}
}

func TestPoolPlanRejectsParityAndDuplicateMembers(t *testing.T) {
	parity := poolDisk("wwn:parity", "/dev/sdc", 300)
	parity.Role = "parity"
	if _, err := NewPoolPlan("pool-1", "media", "/srv/pools/media", []model.Disk{parity}, 1, time.Now().UTC()); err == nil {
		t.Fatal("expected parity disk rejection")
	}
	disk := poolDisk("wwn:a", "/dev/sda", 100)
	if _, err := NewPoolPlan("pool-1", "media", "/srv/pools/media", []model.Disk{disk, disk}, 1, time.Now().UTC()); err == nil {
		t.Fatal("expected duplicate disk rejection")
	}
}

func TestPoolPlanRejectsBranchPathCollision(t *testing.T) {
	first := poolDisk("wwn:a", "/dev/sda", 100)
	second := poolDisk("wwn/a", "/dev/sdb", 100)
	if _, err := NewPoolPlan("pool-1", "media", "/srv/pools/media", []model.Disk{first, second}, 1, time.Now().UTC()); err == nil {
		t.Fatal("expected branch collision rejection")
	}
}

func TestPoolUnmountPlanRequiresTheSameMountedPool(t *testing.T) {
	now := time.Now().UTC()
	pool := model.Pool{ID: "pool-1", Name: "media", MountPath: "/srv/pools/media", Status: model.Healthy}
	plan, err := NewPoolUnmountPlan("unmount-1", pool, 4, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePoolUnmountPlan(plan, []model.Pool{pool}, now.Add(time.Minute), 4); err != nil {
		t.Fatalf("expected matching pool to validate: %v", err)
	}
	if err := ValidatePoolUnmountPlan(plan, nil, now.Add(time.Minute), 4); err == nil {
		t.Fatal("missing mounted pool should fail closed")
	}
}
