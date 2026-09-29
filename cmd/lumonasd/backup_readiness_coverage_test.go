package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestBackupReadinessReportsVerifiedShareCoverageAndRestoreDrill(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_RECOVERY_KEY", "readiness-test-key")
	directory := t.TempDir()
	t.Setenv("LUMONAS_RECOVERY_DIR", directory)

	if _, err := server.store.CreateManagedShare(shares.ManagedShare{
		ID: "share-documents", Name: "Documents", Path: "/srv/pools/documents", Enabled: true,
		Protocols: []shares.Protocol{{Name: "smb"}},
	}); err != nil {
		t.Fatal(err)
	}
	archiveRoot := filepath.Join(directory, "share-data")
	if err := os.MkdirAll(archiveRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveRoot, "note.txt"), []byte("recoverable"), 0600); err != nil {
		t.Fatal(err)
	}
	archive, err := recovery.ArchiveAppdata(archiveRoot, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("readiness-test-key")
	if _, err := server.store.SaveBackupDestination(backup.Destination{
		ID: "local", Name: "Local recovery", Type: backup.DestinationLocal,
		Target: directory, Enabled: true, Retention: backup.DefaultRetention(),
	}, backup.Credentials{}, key); err != nil {
		t.Fatal(err)
	}
	bundle, err := recovery.Create(recovery.Input{
		Manifest:     recovery.Manifest{ConfigSchema: 1, NASUUID: "nas-readiness", Generation: 7, DiskIDs: []string{"disk-1"}},
		DesiredState: []byte(`{"generation":7}`),
		Database:     []byte("SQLite format 3\x00test database"),
		Shares:       []recovery.SharePayload{{ID: "share-documents", Name: "Documents", Path: "/srv/pools/documents", Archive: archive}},
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := recovery.PersistVerified(directory, bundle, key, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	digest, size, err := backup.SHA256File(persisted.LatestPath)
	if err != nil {
		t.Fatal(err)
	}
	finished := time.Now().UTC()
	if err := server.store.SaveBackupRun(backup.Run{ID: "verified-run", Trigger: "manual", Generation: 7, State: "verified", BundlePath: persisted.LatestPath, Checksum: digest, Bytes: size, StartedAt: finished.Add(-time.Minute), FinishedAt: &finished}); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveRestoreDrill(model.RestoreDrill{
		ID: "restore-drill-ok", Trigger: "manual", State: "successful", BundlePath: persisted.LatestPath,
		Generation: 7, StartedAt: finished.Add(-2 * time.Minute), FinishedAt: &finished,
		Verified: true, DatabaseRestored: true, ServicesHealthy: true,
	}); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/backup/readiness", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("readiness returned %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Score  int `json:"score"`
		Layers []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"layers"`
		Coverage []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Score != 100 {
		t.Fatalf("expected all readiness layers current, got score %d: %s", result.Score, response.Body.String())
	}
	for _, item := range result.Coverage {
		if (item.ID == "share-documents" || item.ID == "docker-appdata") && item.Status != "current" {
			t.Fatalf("expected %s coverage to be current, got %q: %s", item.ID, item.Status, response.Body.String())
		}
	}
	for _, id := range []string{"share-documents", "docker-appdata"} {
		found := false
		for _, item := range result.Coverage {
			if item.ID == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s coverage row: %s", id, response.Body.String())
		}
	}
}
