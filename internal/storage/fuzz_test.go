package storage

import "testing"

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
