package storage

import (
	"strings"
	"testing"
)

func TestRenderMountUnitsDiskAndPool(t *testing.T) {
	entries := []MountEntry{
		{Kind: "pool", TargetID: "media", MountPath: "/srv/pools/media", FSType: "fuse.mergerfs", Source: "/srv/disks/wwn_a:/srv/disks/wwn_b", Options: PoolMountOptions, Enabled: true},
		{Kind: "disk", TargetID: "wwn:a", MountPath: "/srv/disks/wwn_a", FSType: "ext4", Source: "UUID=abc-123", Options: DiskMountOptions, Enabled: true},
		{Kind: "disk", TargetID: "wwn:b", MountPath: "/srv/disks/wwn_b", FSType: "xfs", Source: "UUID=def-456", Options: DiskMountOptions, Enabled: true},
	}
	units, err := RenderMountUnits(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d: %v", len(units), unitNames(units))
	}
	diskUnit := units["srv-disks-wwn_a.mount"]
	if !strings.Contains(diskUnit, "What=UUID=abc-123") || !strings.Contains(diskUnit, "Where=/srv/disks/wwn_a") || !strings.Contains(diskUnit, "Type=ext4") {
		t.Fatalf("unexpected disk unit:\n%s", diskUnit)
	}
	poolUnit := units["srv-pools-media.mount"]
	if !strings.Contains(poolUnit, "Requires=srv-disks-wwn_a.mount srv-disks-wwn_b.mount") {
		t.Fatalf("pool unit lacks branch dependencies:\n%s", poolUnit)
	}
	if !strings.Contains(poolUnit, "After=srv-disks-wwn_a.mount srv-disks-wwn_b.mount") {
		t.Fatalf("pool unit lacks After ordering:\n%s", poolUnit)
	}
	if !strings.Contains(poolUnit, "What=/srv/disks/wwn_a:/srv/disks/wwn_b") || !strings.Contains(poolUnit, "Type=fuse.mergerfs") {
		t.Fatalf("unexpected pool unit:\n%s", poolUnit)
	}
}

func unitNames(units map[string]string) []string {
	names := make([]string, 0, len(units))
	for name := range units {
		names = append(names, name)
	}
	return names
}

func TestRenderMountUnitsRejectsUnsafeState(t *testing.T) {
	cases := []struct {
		name    string
		entries []MountEntry
		message string
	}{
		{"non-canonical disk path", []MountEntry{{Kind: "disk", TargetID: "wwn:a", MountPath: "/mnt/a", FSType: "ext4", Source: "UUID=x", Enabled: true}}, "canonical"},
		{"unsupported filesystem", []MountEntry{{Kind: "disk", TargetID: "wwn:a", MountPath: "/srv/disks/wwn_a", FSType: "btrfs", Source: "UUID=x", Enabled: true}}, "allow-listed"},
		{"missing UUID source", []MountEntry{{Kind: "disk", TargetID: "wwn:a", MountPath: "/srv/disks/wwn_a", FSType: "ext4", Source: "/dev/sda", Enabled: true}}, "UUID="},
		{"pool outside /srv/pools", []MountEntry{{Kind: "pool", TargetID: "media", MountPath: "/srv/poolz/media", FSType: "fuse.mergerfs", Source: "/srv/disks/a", Enabled: true}}, "canonical"},
		{"foreign pool branch", []MountEntry{{Kind: "pool", TargetID: "media", MountPath: "/srv/pools/media", FSType: "fuse.mergerfs", Source: "/mnt/data", Enabled: true}}, "invalid branch"},
		{"duplicate mount path", []MountEntry{{Kind: "disk", TargetID: "wwn:a", MountPath: "/srv/disks/wwn_a", FSType: "ext4", Source: "UUID=x", Enabled: true}, {Kind: "disk", TargetID: "wwn:a", MountPath: "/srv/disks/wwn_a", FSType: "xfs", Source: "UUID=y", Enabled: true}}, "duplicated"},
		{"unknown kind", []MountEntry{{Kind: "raid", TargetID: "x", MountPath: "/srv/disks/x", FSType: "ext4", Source: "UUID=x", Enabled: true}}, "kind"},
	}
	for _, testCase := range cases {
		if _, err := RenderMountUnits(testCase.entries); err == nil || !strings.Contains(err.Error(), testCase.message) {
			t.Fatalf("%s: expected rejection containing %q, got %v", testCase.name, testCase.message, err)
		}
	}
}

func TestMountUnitNameEscapingIsRestricted(t *testing.T) {
	if name, err := MountUnitName("/srv/disks/wwn_Test"); err != nil || name != "srv-disks-wwn_Test.mount" {
		t.Fatalf("unexpected unit name %q err %v", name, err)
	}
	if name, err := MountUnitName("/srv/pools/media"); err != nil || name != "srv-pools-media.mount" {
		t.Fatalf("unexpected pool unit name %q err %v", name, err)
	}
	for _, unsafe := range []string{"/srv/disks/../etc", "/srv/disks/a b", "/etc/passwd", "/srv/disks/"} {
		if _, err := MountUnitName(unsafe); err == nil {
			t.Fatalf("expected rejection for %q", unsafe)
		}
	}
}
