package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/foldersync"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestTwoWayFolderSyncRequiresReviewedPlanAndSavesBaseline(t *testing.T) {
	server := testServer(t)
	leftRoot := filepath.Join(t.TempDir(), "left")
	rightRoot := filepath.Join(t.TempDir(), "right")
	if err := os.MkdirAll(leftRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rightRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leftRoot, "one.txt"), []byte("shared"), 0o640); err != nil {
		t.Fatal(err)
	}
	for _, share := range []shares.ManagedShare{
		{ID: "share-left", Name: "Left", Path: leftRoot, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}},
		{ID: "share-right", Name: "Right", Path: rightRoot, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}},
	} {
		if _, err := server.store.CreateManagedShare(share); err != nil {
			t.Fatal(err)
		}
	}
	task := foldersync.Task{ID: "sync-bilateral", Name: "Family folders", Direction: "two-way", Source: foldersync.Endpoint{Kind: "share", ShareID: "share-left"}, Destination: foldersync.Endpoint{Kind: "share", ShareID: "share-right"}, Mode: "copy", ScheduleKind: "manual"}
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveFolderSyncTask(task); err != nil {
		t.Fatal(err)
	}
	preview := httptest.NewRecorder()
	server.routes().ServeHTTP(preview, httptest.NewRequest(http.MethodPost, "/api/v1/folder-sync/tasks/sync-bilateral/preview", nil))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview returned %d: %s", preview.Code, preview.Body.String())
	}
	var result struct {
		Plan                 foldersync.Plan `json:"plan"`
		PlanHash             string          `json:"planHash"`
		RequiresConfirmation bool            `json:"requiresConfirmation"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.RequiresConfirmation || result.PlanHash == "" || result.Plan.Files != 1 {
		t.Fatalf("two-way preview did not return a confirmable transfer plan: %#v", result)
	}
	startRequest := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/folder-sync/tasks/sync-bilateral/run", bytes.NewBufferString(body)))
		return response
	}
	unconfirmed := startRequest(`{"planHash":"` + result.PlanHash + `"}`)
	if unconfirmed.Code != http.StatusConflict {
		t.Fatalf("unconfirmed run returned %d: %s", unconfirmed.Code, unconfirmed.Body.String())
	}
	confirmed := startRequest(`{"planHash":"` + result.PlanHash + `","confirmTwoWay":true}`)
	if confirmed.Code != http.StatusAccepted {
		t.Fatalf("confirmed run returned %d: %s", confirmed.Code, confirmed.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := server.store.FolderSyncRuns(task.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) > 0 && runs[0].State != "running" {
			if runs[0].State != "successful" {
				t.Fatalf("two-way run finished as %q: %s", runs[0].State, runs[0].Error)
			}
			baseline, initialized, err := server.store.FolderSyncBaseline(task.ID)
			if err != nil || !initialized || len(baseline) != 1 {
				t.Fatalf("successful run did not save its common baseline: %#v %v %v", baseline, initialized, err)
			}
			copied, err := os.ReadFile(filepath.Join(rightRoot, "one.txt"))
			if err != nil || string(copied) != "shared" {
				t.Fatalf("file was not copied to second endpoint: %q %v", copied, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("two-way run did not finish before timeout")
}
