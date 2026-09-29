package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/store"
)

func TestStorageSnapshotRestoreCopiesSelectedEntryWithoutOverwrite(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, err := server.store.ManagedShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	name := "20260928-010203"
	snapshotRoot := share.Path + ".snapshots/" + name
	if err := os.MkdirAll(filepath.Join(snapshotRoot, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshotRoot, "docs", "notes.txt"), []byte("snapshot version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share.Path, "restored"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: share.Path, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"shareId":"` + shareID + `","snapshotPath":"docs","targetPath":"restored","names":["notes.txt"]}`
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/snapshots/"+snapshot.ID+"/restore", strings.NewReader(body)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("restore was not queued: %d %s", response.Code, response.Body.String())
	}
	target := filepath.Join(share.Path, "restored", "notes.txt")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, readErr := os.ReadFile(target); readErr == nil {
			if string(data) != "snapshot version" {
				t.Fatalf("restored wrong contents: %q", data)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("snapshot file was not restored")
}

func TestStorageSnapshotRestoreCreatesSafeRecoveryTree(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, err := server.store.ManagedShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	name := "20260929-010203"
	snapshotRoot := filepath.Join(share.Path+".snapshots", name)
	if err := os.MkdirAll(filepath.Join(snapshotRoot, "Photos", "2026"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshotRoot, "Photos", "2026", "family.jpg"), []byte("recoverable photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: share.Path, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	targetPath := "Recovered from " + name + "/Photos/2026"
	body := `{"shareId":"` + shareID + `","snapshotPath":"Photos/2026","targetPath":"` + targetPath + `","createTargetDirectories":true,"names":["family.jpg"]}`
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/snapshots/"+snapshot.ID+"/restore", strings.NewReader(body)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("restore was not queued: %d %s", response.Code, response.Body.String())
	}
	target := filepath.Join(share.Path, filepath.FromSlash(targetPath), "family.jpg")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, readErr := os.ReadFile(target); readErr == nil {
			if string(data) != "recoverable photo" {
				t.Fatalf("restored wrong contents: %q", data)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("snapshot file was not restored into the recovered folder tree")
}

func TestStorageSnapshotRestoreRejectsOverwriteAndOtherShare(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, err := server.store.ManagedShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	name := "20260928-020304"
	snapshotRoot := share.Path + ".snapshots/" + name
	if err := os.MkdirAll(snapshotRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshotRoot, "readme.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share.Path, "readme.txt"), []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: share.Path, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"shareId":"` + shareID + `","snapshotPath":"","targetPath":"","names":["readme.txt"]}`
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/snapshots/"+snapshot.ID+"/restore", strings.NewReader(body)))
	if response.Code != http.StatusConflict {
		t.Fatalf("overwrite was not rejected: %d %s", response.Code, response.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(share.Path, "readme.txt"))
	if err != nil || string(data) != "current" {
		t.Fatalf("existing file changed: %q err=%v", data, err)
	}
	otherShareID := "other-test-share"
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{ID: otherShareID, Name: "OtherFiles", Path: t.TempDir(), Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}); err != nil {
		t.Fatal(err)
	}
	body = `{"shareId":"` + otherShareID + `","snapshotPath":"","targetPath":"safe","names":["readme.txt"]}`
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/snapshots/"+snapshot.ID+"/restore", strings.NewReader(body)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("restore crossed managed share boundary: %d %s", response.Code, response.Body.String())
	}
}
