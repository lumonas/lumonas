package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackupCompatibilityContract(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_RECOVERY_KEY", "compatibility-key")

	save := httptest.NewRecorder()
	server.routes().ServeHTTP(save, httptest.NewRequest(http.MethodPost, "/api/v1/backups/destinations", strings.NewReader(`{"id":"compat","name":"Local recovery","type":"local","target":"/tmp/lumonas-recovery","enabled":true}`)))
	if save.Code != http.StatusOK {
		t.Fatalf("save destination failed: %d %s", save.Code, save.Body.String())
	}
	t.Logf("saved destination: %s", save.Body.String())

	read := func(method, endpoint string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(method, endpoint, nil))
		return response
	}
	readiness := read(http.MethodGet, "/api/v1/backup/readiness")
	if readiness.Code != http.StatusOK || !strings.Contains(readiness.Body.String(), `"layers"`) {
		t.Fatalf("unexpected readiness response: %d %s", readiness.Code, readiness.Body.String())
	}
	destinations := read(http.MethodGet, "/api/v1/backup/destinations")
	if destinations.Code != http.StatusOK || !strings.Contains(destinations.Body.String(), `"id":"compat"`) || !strings.Contains(destinations.Body.String(), `"type":"nas"`) || !strings.Contains(destinations.Body.String(), `"status":"attention"`) {
		t.Fatalf("unexpected destinations response: %d %s", destinations.Code, destinations.Body.String())
	}
	jobs := read(http.MethodGet, "/api/v1/backup/jobs")
	if jobs.Code != http.StatusOK || !strings.Contains(jobs.Body.String(), `"destinationId":"compat"`) {
		t.Fatalf("unexpected jobs response: %d %s", jobs.Code, jobs.Body.String())
	}
	generations := read(http.MethodGet, "/api/v1/backup/generations")
	if generations.Code != http.StatusOK || !strings.Contains(generations.Body.String(), `"status":"committed"`) {
		t.Fatalf("unexpected generations response: %d %s", generations.Code, generations.Body.String())
	}
	restore := read(http.MethodGet, "/api/v1/backup/restore/plan")
	if restore.Code != http.StatusOK || !strings.Contains(restore.Body.String(), `"dataDisksNote"`) {
		t.Fatalf("unexpected restore plan response: %d %s", restore.Code, restore.Body.String())
	}
}
