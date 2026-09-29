package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/foldersync"
)

func TestFolderSyncPersistenceAndInterruptedRunReconciliation(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	task := foldersync.Task{ID: "sync-1", Name: "Archive", Source: foldersync.Endpoint{Kind: "share", ShareID: "share-1"}, Destination: foldersync.Endpoint{Kind: "share", ShareID: "share-2"}, Mode: "copy", ScheduleKind: "manual"}
	if err := database.SaveFolderSyncTask(task); err != nil {
		t.Fatal(err)
	}
	saved, err := database.FolderSyncTask(task.ID)
	if err != nil || saved.Name != task.Name {
		t.Fatalf("task roundtrip failed: %#v %v", saved, err)
	}
	if baseline, initialized, err := database.FolderSyncBaseline(task.ID); err != nil || initialized || baseline != nil {
		t.Fatalf("unexpected uninitialized baseline: %#v %v %v", baseline, initialized, err)
	}
	baselineEntries := map[string]foldersync.Entry{"hello.txt": {Path: "hello.txt", Size: 5, ModTime: time.Now().UTC()}}
	if err := database.SaveFolderSyncBaseline(task.ID, baselineEntries); err != nil {
		t.Fatal(err)
	}
	loadedBaseline, initialized, err := database.FolderSyncBaseline(task.ID)
	if err != nil || !initialized || loadedBaseline["hello.txt"].Size != 5 {
		t.Fatalf("baseline did not roundtrip: %#v %v %v", loadedBaseline, initialized, err)
	}
	if err := database.SaveFolderSyncRun(foldersync.Run{ID: "run-1", TaskID: task.ID, State: "running", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := database.ReconcileInterruptedFolderSyncRuns(); err != nil {
		t.Fatal(err)
	}
	runs, err := database.FolderSyncRuns(task.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].State != "failed" || runs[0].Error == "" {
		t.Fatalf("interrupted run was not reconciled: %#v %v", runs, err)
	}
	if err := database.DeleteFolderSyncTask(task.ID); err != nil {
		t.Fatal(err)
	}
	if baseline, initialized, err := database.FolderSyncBaseline(task.ID); err != nil || initialized || baseline != nil {
		t.Fatalf("deleting task should clear its baseline: %#v %v %v", baseline, initialized, err)
	}
}
