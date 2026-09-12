package store

import (
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/storage"
)

func TestMountEntriesRoundTripAndChangeDetection(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "lumonas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	entries, err := db.MountEntries()
	if err != nil || len(entries) != 0 {
		t.Fatalf("expected empty mount state: %#v err %v", entries, err)
	}
	desired := []storage.MountEntry{
		{Kind: "disk", TargetID: "wwn:a", MountPath: "/srv/disks/wwn_a", FSType: "ext4", Source: "UUID=abc", Options: storage.DiskMountOptions, Enabled: true},
		{Kind: "pool", TargetID: "media", MountPath: "/srv/pools/media", FSType: "fuse.mergerfs", Source: "/srv/disks/wwn_a", Options: storage.PoolMountOptions, Enabled: true},
	}
	if changed, err := db.MountEntriesChanged(desired); err != nil || !changed {
		t.Fatalf("expected changed state: changed=%v err=%v", changed, err)
	}
	if err := db.SaveMountEntries(desired); err != nil {
		t.Fatal(err)
	}
	saved, err := db.MountEntries()
	if err != nil || len(saved) != 2 {
		t.Fatalf("unexpected saved entries: %#v err %v", saved, err)
	}
	if saved[0].MountPath != "/srv/disks/wwn_a" || saved[1].MountPath != "/srv/pools/media" {
		t.Fatalf("entries are not ordered by mount path: %#v", saved)
	}
	if changed, err := db.MountEntriesChanged(desired); err != nil || changed {
		t.Fatalf("unchanged state should not require regeneration: changed=%v err=%v", changed, err)
	}
	if err := db.SaveMountEntries(nil); err != nil {
		t.Fatal(err)
	}
	if entries, err := db.MountEntries(); err != nil || len(entries) != 0 {
		t.Fatalf("expected pruned mount state: %#v err %v", entries, err)
	}
}

func TestSaveMountEntriesRejectsUnsafeState(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "lumonas.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	unsafe := []storage.MountEntry{{Kind: "disk", TargetID: "wwn:a", MountPath: "/etc/evil", FSType: "ext4", Source: "UUID=abc", Enabled: true}}
	if err := db.SaveMountEntries(unsafe); err == nil {
		t.Fatal("expected unsafe mount state rejection")
	}
	if entries, _ := db.MountEntries(); len(entries) != 0 {
		t.Fatalf("rejected state must not persist: %#v", entries)
	}
}
