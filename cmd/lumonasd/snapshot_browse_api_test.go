package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/store"
)

func TestStorageSnapshotFilesRequiresBtrfs(t *testing.T) {
	server, _ := lanTestServer(t)
	zfs, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "zfs", Source: "tank/media", Name: "nightly-20260913T100000Z"})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/snapshots/"+zfs.ID+"/files", nil))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for zfs snapshot, got %d: %s", response.Code, response.Body.String())
	}
}

func TestStorageSnapshotFilesListsBrokerEntries(t *testing.T) {
	server, requests := lanTestServer(t)
	// Replace the seam with one that returns a real broker payload.
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		*requests = append(*requests, request)
		return nil
	}
	// The seam cannot return data; drive the decoder directly to prove
	// payload handling, then confirm the HTTP path surfaces an empty list.
	record, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pool", Name: "nightly-20260913T100000Z"})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/snapshots/"+record.ID+"/files", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Path    string                `json:"path"`
		Entries []model.SnapshotEntry `json:"entries"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Entries) != 0 {
		t.Fatalf("seam payload must decode to empty entries: %#v", payload)
	}
	if len(*requests) != 1 || (*requests)[0].Operation != "snapshot.browse" {
		t.Fatalf("broker was not asked to browse: %#v", *requests)
	}

	decoded, total := snapshotEntries(privileged.Response{OK: true, Data: map[string]any{
		"total": float64(3),
		"entries": []any{
			map[string]any{"name": "media", "directory": true, "sizeBytes": float64(96), "modifiedAt": "2026-09-13T10:00:00Z"},
			map[string]any{"name": "README.txt", "directory": false, "sizeBytes": float64(8), "modifiedAt": "2026-09-13T10:00:00Z"},
			map[string]any{"directory": true},
		},
	}})
	if len(decoded) != 2 || total != 3 || decoded[0].Name != "media" || !decoded[0].Directory || decoded[1].Name != "README.txt" {
		t.Fatalf("unexpected decoded entries: %#v total=%d", decoded, total)
	}
}

func TestStorageSnapshotFilesMissingSnapshot(t *testing.T) {
	server, _ := lanTestServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/snapshots/snap-absent/files", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestStorageSnapshotCompareRunsThroughPrivilegedWorker(t *testing.T) {
	server, requests := lanTestServer(t)
	record, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pool", Name: "nightly-20260913T100000Z"})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/snapshots/"+record.ID+"/compare", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if len(*requests) != 1 || (*requests)[0].Operation != "snapshot.compare" || (*requests)[0].RequestedState["name"] != record.Name {
		t.Fatalf("comparison did not use the storage worker: %#v", *requests)
	}
}
