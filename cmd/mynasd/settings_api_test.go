package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettingsAPIUsesPersistedAllowListedSections(t *testing.T) {
	server := testServer(t)
	get := httptest.NewRecorder()
	server.routes().ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"runtime"`) || !strings.Contains(get.Body.String(), `"security"`) {
		t.Fatalf("unexpected settings response: %d %s", get.Code, get.Body.String())
	}

	update := httptest.NewRecorder()
	server.routes().ServeHTTP(update, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"section":"runtime","patch":{"writeProfile":"maximum"}}`)))
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"writeProfile":"maximum"`) {
		t.Fatalf("settings update failed: %d %s", update.Code, update.Body.String())
	}

	bad := httptest.NewRecorder()
	server.routes().ServeHTTP(bad, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"section":"runtime","patch":{"shell":"rm -rf"}}`)))
	if bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected allow-list rejection, got %d: %s", bad.Code, bad.Body.String())
	}

	persisted := httptest.NewRecorder()
	server.routes().ServeHTTP(persisted, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if persisted.Code != http.StatusOK || !strings.Contains(persisted.Body.String(), `"writeProfile":"maximum"`) {
		t.Fatalf("settings update did not persist: %d %s", persisted.Code, persisted.Body.String())
	}
}

func TestUpdateCheckQueuesAndCompletesJob(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/updates/check", nil))
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"type":"updates.check"`) {
		t.Fatalf("unexpected update check response: %d %s", response.Code, response.Body.String())
	}
	var queued struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&queued); err != nil || queued.ID == "" {
		t.Fatalf("invalid queued job: %#v err=%v", queued, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		job, err := server.store.Job(queued.ID)
		if err == nil && job.State == "successful" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, err := server.store.Job(queued.ID)
	if err != nil || job.State != "successful" {
		t.Fatalf("update check did not complete: %#v err=%v", job, err)
	}
}
