package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotReplicationTaskPersistenceAndResult(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "replication.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveReplicationPeer(ReplicationPeer{ID: "peer-1", Name: "Remote", URL: "https://remote.test"}, []byte("peer-token-ciphertext")); err != nil {
		t.Fatal(err)
	}
	task := SnapshotReplicationTask{ID: "task-1", PeerID: "peer-1", Name: "Photos", SourceShareID: "share-source", DestinationShareID: "share-target", ScheduleKind: "weekly", TimeOfDay: "03:15", Weekday: "saturday"}
	if err := database.SaveSnapshotReplicationTask(task, []byte("encrypted receive credential")); err != nil {
		t.Fatal(err)
	}
	loaded, ciphertext, err := database.SnapshotReplicationTask(task.ID)
	if err != nil || loaded.ScheduleKind != "weekly" || loaded.Weekday != "saturday" || string(ciphertext) != "encrypted receive credential" {
		t.Fatalf("task roundtrip failed: %#v %q %v", loaded, ciphertext, err)
	}
	attempted := time.Date(2026, 9, 29, 1, 2, 3, 0, time.UTC)
	if err := database.RecordSnapshotReplicationAttempt(task.ID, attempted); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordSnapshotReplicationResult(task.ID, "replica-1", "network failure"); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = database.SnapshotReplicationTask(task.ID)
	if err != nil || loaded.LastAttemptAt == nil || !loaded.LastAttemptAt.Equal(attempted) || loaded.LastSnapshotName != "" || loaded.LastError != "network failure" {
		t.Fatalf("failed run changed successful replication state: %#v %v", loaded, err)
	}
	if err := database.RecordSnapshotReplicationResult(task.ID, "replica-2", ""); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = database.SnapshotReplicationTask(task.ID)
	if err != nil || loaded.LastSnapshotName != "replica-2" || loaded.LastSyncAt == nil || loaded.LastError != "" {
		t.Fatalf("successful run was not recorded: %#v %v", loaded, err)
	}
	if err := database.DeleteSnapshotReplicationTask(task.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.SnapshotReplicationTask(task.ID); err == nil {
		t.Fatal("deleted replication task still exists")
	}
}
