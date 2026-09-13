package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/store"
)

func TestStorageSnapshotCreatePersistsRecord(t *testing.T) {
	server := testServer(t)
	var seen privileged.Request
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		seen = request
		return nil
	}

	body := `{"kind":"btrfs","source":"/srv/pool","label":"nightly"}`
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/snapshots", strings.NewReader(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var record store.StorageSnapshotRecord
	if err := json.NewDecoder(response.Body).Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.Kind != "btrfs" || record.Source != "/srv/pool" || !strings.HasPrefix(record.Name, "nightly-") {
		t.Fatalf("unexpected record: %#v", record)
	}
	if seen.Operation != "snapshot.create" || !seen.Confirmed || seen.OperationID == "" {
		t.Fatalf("unexpected broker request: %#v", seen)
	}
	persisted, err := server.store.StorageSnapshots("/srv/pool", 10)
	if err != nil || len(persisted) != 1 {
		t.Fatalf("snapshot was not persisted: %#v err=%v", persisted, err)
	}
}

func TestStorageSnapshotCreateValidatesInput(t *testing.T) {
	server := testServer(t)
	server.brokerExec = func(context.Context, privileged.Request) error { return nil }

	cases := map[string]string{
		"relative source": `{"kind":"btrfs","source":"relative"}`,
		"bad kind":        `{"kind":"ext4","source":"/srv/pool"}`,
		"bad label":       `{"kind":"btrfs","source":"/srv/pool","label":"has space"}`,
	}
	for label, body := range cases {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/snapshots", strings.NewReader(body)))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: expected 422, got %d: %s", label, response.Code, response.Body.String())
		}
	}
}

func TestStorageSnapshotDeleteRemovesPersistedRecord(t *testing.T) {
	server := testServer(t)
	var seen privileged.Request
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		seen = request
		return nil
	}
	saved, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "zfs", Source: "tank/media", Name: "nightly-20260913T100000Z"})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.safetyMu.Lock()
	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	server.safetyMu.Unlock()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/storage/snapshots/"+saved.ID, strings.NewReader(`{"reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if seen.Operation != "snapshot.delete" || seen.RequestedState["name"] != "nightly-20260913T100000Z" {
		t.Fatalf("unexpected broker request: %#v", seen)
	}
	remaining, err := server.store.StorageSnapshots("tank/media", 10)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("record was not removed: %#v err=%v", remaining, err)
	}

	missing := httptest.NewRecorder()
	server.routes().ServeHTTP(missing, httptest.NewRequest(http.MethodDelete, "/api/v1/storage/snapshots/snap-missing", strings.NewReader(`{"reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown snapshot, got %d", missing.Code)
	}
}

func TestStorageSnapshotDeleteRequiresSafetyUnlock(t *testing.T) {
	server := testServer(t)
	saved, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "zfs", Source: "tank/media", Name: "nightly"})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/storage/snapshots/"+saved.ID, strings.NewReader(`{"reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if response.Code != http.StatusLocked {
		t.Fatalf("expected 423 while storage safety is locked, got %d: %s", response.Code, response.Body.String())
	}
}

func TestStorageSnapshotsList(t *testing.T) {
	server := testServer(t)
	if _, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pool", Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "zfs", Source: "tank", Name: "b"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/snapshots?source=tank", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var records []store.StorageSnapshotRecord
	if err := json.NewDecoder(response.Body).Decode(&records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Name != "b" {
		t.Fatalf("unexpected listing: %#v", records)
	}
}

func TestStorageSnapshotFilesBrowsesBtrfsThroughPrivilegedBroker(t *testing.T) {
	server := testServer(t)
	saved, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pool", Name: "nightly-20260913T100000Z"})
	if err != nil {
		t.Fatal(err)
	}
	var seen privileged.Request
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		seen = request
		return privileged.Response{OK: true, Data: map[string]any{
			"total":   float64(1),
			"entries": []any{map[string]any{"name": "movies", "directory": true, "sizeBytes": float64(128), "modifiedAt": "2026-09-13T10:00:00Z"}},
		}}, nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/snapshots/"+saved.ID+"/files?path=media", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Path    string                `json:"path"`
		Entries []model.SnapshotEntry `json:"entries"`
		Total   int                   `json:"total"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Path != "media" || payload.Total != 1 || len(payload.Entries) != 1 || payload.Entries[0].Name != "movies" || !payload.Entries[0].Directory {
		t.Fatalf("unexpected browse response: %#v", payload)
	}
	if seen.Operation != "snapshot.browse" || seen.RequestedState["subpath"] != "media" || !seen.Confirmed {
		t.Fatalf("unexpected browse broker request: %#v", seen)
	}
}

func TestStorageSnapshotFilesRejectsZFS(t *testing.T) {
	server := testServer(t)
	saved, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "zfs", Source: "tank/media", Name: "nightly-20260913T100000Z"})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/snapshots/"+saved.ID+"/files", nil))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for zfs browse, got %d: %s", response.Code, response.Body.String())
	}
}
