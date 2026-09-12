package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
)

func TestBackupDestinationAPIDoesNotReturnCredentials(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_RECOVERY_KEY", "backup-key")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/backups/destinations", strings.NewReader(`{"id":"local","name":"Local","type":"local","target":"/tmp/mynas-backups","enabled":true,"credentials":{"secretKey":"do-not-return"}}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save destination status %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "do-not-return") {
		t.Fatal("destination response leaked credentials")
	}
	var destination backup.Destination
	if err := json.NewDecoder(response.Body).Decode(&destination); err != nil {
		t.Fatal(err)
	}
	if !destination.CredentialsConfigured {
		t.Fatal("expected credential marker")
	}

	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/backups/destinations", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"local"`) {
		t.Fatalf("unexpected destination list %d: %s", list.Code, list.Body.String())
	}
}

func TestBackupStatusReportsMissingConfiguration(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_RECOVERY_KEY", "")
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/backups/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "no backup destination") {
		t.Fatalf("unexpected backup status %d: %s", response.Code, response.Body.String())
	}
}

func TestBackupStatusReportsVerifiedCopies(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_RECOVERY_KEY", "backup-health-key")
	if _, err := server.store.SaveBackupDestination(backup.Destination{ID: "local", Name: "Local", Type: backup.DestinationLocal, Target: "/tmp/mynas-backups", Enabled: true, Retention: backup.DefaultRetention()}, backup.Credentials{}, []byte("backup-health-key")); err != nil {
		t.Fatal(err)
	}
	run := backup.Run{ID: "run-1", Trigger: "scheduled", Generation: 3, State: "verified", StartedAt: time.Now().UTC()}
	if err := server.store.SaveBackupRun(run); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveBackupCopy(backup.Copy{ID: "copy-1", RunID: run.ID, DestinationID: "local", Object: "recovery/run-1.mrb", Checksum: "abc", Bytes: 42, State: "verified", Verified: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/backups/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"healthyCopies":1`) {
		t.Fatalf("unexpected backup health %d: %s", response.Code, response.Body.String())
	}
}
