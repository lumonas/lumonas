package storage

import (
	"strings"
	"testing"
)

func TestRenderMountUnitsNormalizesDiskAndPoolDependencies(t *testing.T) {
	entries := []MountEntry{
		{Kind: "pool", TargetID: "media", MountPath: "/srv/pools/media", FSType: "fuse.mergerfs", Source: "/srv/disks/wwn-b:/srv/disks/wwn-a"},
		{Kind: "disk", TargetID: "wwn-a", MountPath: "/srv/disks/wwn-a", FSType: "ext4", Source: "UUID=aaa"},
		{Kind: "disk", TargetID: "wwn-b", MountPath: "/srv/disks/wwn-b", FSType: "xfs", Source: "UUID=bbb", Enabled: true},
	}
	units, err := RenderMountUnits(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 3 {
		t.Fatalf("expected three mount units, got %d", len(units))
	}
	pool := units["srv-pools-media.mount"]
	for _, expected := range []string{"Requires=srv-disks-wwn-a.mount srv-disks-wwn-b.mount", "After=srv-disks-wwn-a.mount srv-disks-wwn-b.mount", "What=/srv/disks/wwn-b:/srv/disks/wwn-a", "Type=fuse.mergerfs", "category.create=mfs"} {
		if !strings.Contains(pool, expected) {
			t.Fatalf("pool unit missing %q:\n%s", expected, pool)
		}
	}
	disk := units["srv-disks-wwn-a.mount"]
	if !strings.Contains(disk, "Options=defaults,nofail") || !strings.Contains(disk, "Where=/srv/disks/wwn-a") {
		t.Fatalf("disk unit missing defaults:\n%s", disk)
	}
}

func TestNormalizeMountEntriesRejectsAmbiguousOrUnsafeState(t *testing.T) {
	cases := []MountEntry{
		{Kind: "disk", TargetID: "", MountPath: "/srv/disks/", FSType: "ext4", Source: "UUID=aaa"},
		{Kind: "disk", TargetID: "wwn-a", MountPath: "/srv/disks/other", FSType: "ext4", Source: "UUID=aaa"},
		{Kind: "disk", TargetID: "wwn-a", MountPath: "/srv/disks/wwn-a", FSType: "ext4", Source: "/dev/sda"},
		{Kind: "pool", TargetID: "media", MountPath: "/srv/pools/media", FSType: "fuse.mergerfs", Source: "/srv/disks/../secret"},
	}
	for _, entry := range cases {
		if _, err := NormalizeMountEntries([]MountEntry{entry}); err == nil {
			t.Fatalf("unsafe mount entry was accepted: %#v", entry)
		}
	}
	duplicate := MountEntry{Kind: "disk", TargetID: "wwn-a", MountPath: "/srv/disks/wwn-a", FSType: "ext4", Source: "UUID=aaa"}
	if _, err := NormalizeMountEntries([]MountEntry{duplicate, duplicate}); err == nil {
		t.Fatal("duplicate mount path was accepted")
	}
}
