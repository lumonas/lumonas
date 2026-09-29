package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStorageSnapshotRetentionLockRoundTrip(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "snapshots.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	created := time.Now().UTC().Truncate(time.Second)
	protectedUntil := created.Add(30 * 24 * time.Hour)
	saved, err := database.SaveStorageSnapshot(StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pools/docs", Name: "nightly", CreatedAt: created, ProtectedUntil: &protectedUntil})
	if err != nil {
		t.Fatal(err)
	}
	items, err := database.StorageSnapshots(saved.Source, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("snapshot list failed: %#v %v", items, err)
	}
	if items[0].ProtectedUntil == nil || !items[0].ProtectedUntil.Equal(protectedUntil) {
		t.Fatalf("retention lock was not persisted: %#v", items[0])
	}
}

func TestStorageSnapshotSchemaAddsRetentionLockToExistingDatabase(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.db.Exec(`DROP TABLE storage_snapshots; CREATE TABLE storage_snapshots(id TEXT PRIMARY KEY,kind TEXT NOT NULL,source TEXT NOT NULL,name TEXT NOT NULL,label TEXT,origin TEXT NOT NULL DEFAULT 'manual',created_at TEXT NOT NULL,UNIQUE(kind,source,name)); INSERT INTO storage_snapshots(id,kind,source,name,label,origin,created_at) VALUES('legacy','btrfs','/srv/pools/docs','old',NULL,'manual',?)`, time.Now().UTC().Format(timeFormat))
	if err != nil {
		t.Fatal(err)
	}
	items, err := database.StorageSnapshots("/srv/pools/docs", 10)
	if err != nil || len(items) != 1 || items[0].ID != "legacy" || items[0].ProtectedUntil != nil {
		t.Fatalf("legacy snapshot row did not survive the lock migration: %#v %v", items, err)
	}
	lockedUntil := time.Now().UTC().Add(time.Hour)
	if _, err := database.SaveStorageSnapshot(StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pools/docs", Name: "new", ProtectedUntil: &lockedUntil}); err != nil {
		t.Fatal(err)
	}
}
