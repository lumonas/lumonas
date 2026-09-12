package store

import (
	"testing"

	"github.com/lumonas/lumonas/internal/storage"
)

func TestMountEntriesPersistAndDetectChanges(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	entries := []storage.MountEntry{{
		Kind: "disk", TargetID: "wwn-a", MountPath: "/srv/disks/wwn-a", FSType: "ext4", Source: "UUID=aaa", Enabled: true,
	}}
	if err := database.SaveMountEntries(entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := database.MountEntries()
	if err != nil || len(loaded) != 1 || loaded[0].Source != "UUID=aaa" {
		t.Fatalf("unexpected persisted mounts: %#v err=%v", loaded, err)
	}
	if changed, err := database.MountEntriesChanged(entries); err != nil || changed {
		t.Fatalf("identical mount state reported changed=%v err=%v", changed, err)
	}
	entries[0].Source = "UUID=bbb"
	if changed, err := database.MountEntriesChanged(entries); err != nil || !changed {
		t.Fatalf("changed mount state reported changed=%v err=%v", changed, err)
	}
}
