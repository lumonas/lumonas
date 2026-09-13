package storage

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func FuzzValidateSnapraidConfig(f *testing.F) {
	valid, err := RenderSnapraidConfig("wwn:parity", []string{"wwn:data"})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add("data d1 /srv/disks/wwn_data\nexec rm -rf /\n")
	f.Fuzz(func(t *testing.T, config string) {
		_ = ValidateSnapraidConfig(config)
	})
}

func FuzzRenderMountUnits(f *testing.F) {
	f.Add("wwn:data", "UUID=test", "ext4")
	f.Add("../../etc", "UUID=bad", "xfs")
	f.Fuzz(func(t *testing.T, diskID, source, filesystem string) {
		_, _ = RenderMountUnits([]MountEntry{{Kind: "disk", TargetID: diskID, MountPath: DiskBranchPath(diskID), FSType: filesystem, Source: source, Enabled: true}})
	})
}

func FuzzStoragePlanValidation(f *testing.F) {
	f.Add("wwn:test", "ext4", "/srv/disks/wwn_test")
	f.Add("", "btrfs", "../../etc")
	f.Fuzz(func(t *testing.T, diskID, filesystem, mountPath string) {
		disk := model.Disk{ID: diskID, CurrentPath: "/dev/fuzz", SizeBytes: 100, Role: "unknown"}
		plan, err := NewPlan("fuzz-operation", ActionCreate, disk, 1, time.Now().UTC())
		if err != nil {
			return
		}
		plan.RequestedState = map[string]any{"filesystem": filesystem, "mountPath": mountPath}
		plan.PlanHash = Hash(plan)
		_ = Validate(plan, disk, time.Now().UTC(), 1)
	})
}
