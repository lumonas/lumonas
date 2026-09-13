package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

func TestBackupDestinationAPIDoesNotReturnCredentials(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_RECOVERY_KEY", "backup-key")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/backups/destinations", strings.NewReader(`{"id":"local","name":"Local","type":"local","target":"/tmp/lumonas-backups","enabled":true,"credentials":{"secretKey":"do-not-return"}}`))
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
	t.Setenv("LUMONAS_RECOVERY_KEY", "")
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/backups/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "no backup destination") {
		t.Fatalf("unexpected backup status %d: %s", response.Code, response.Body.String())
	}
}

func TestBackupStatusReportsVerifiedCopies(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_RECOVERY_KEY", "backup-health-key")
	if _, err := server.store.SaveBackupDestination(backup.Destination{ID: "local", Name: "Local", Type: backup.DestinationLocal, Target: "/tmp/lumonas-backups", Enabled: true, Retention: backup.DefaultRetention()}, backup.Credentials{}, []byte("backup-health-key")); err != nil {
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

func TestReconcileInterruptedBackupsFailsClosed(t *testing.T) {
	server := testServer(t)
	run := backup.Run{ID: "interrupted", Trigger: "scheduled", Generation: 4, State: "running", StartedAt: time.Now().UTC().Add(-time.Minute)}
	if err := server.store.SaveBackupRun(run); err != nil {
		t.Fatal(err)
	}
	server.reconcileInterruptedBackups()
	runs, err := server.store.BackupRuns(1)
	if err != nil || len(runs) != 1 || runs[0].State != "failed" || runs[0].Error != interruptedBackupReason {
		t.Fatalf("interrupted backup was not reconciled: %#v %v", runs, err)
	}
}

func TestBackupStatusReportsInterruptedFailure(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveBackupRun(backup.Run{ID: "failed", Trigger: "scheduled", Generation: 4, State: "failed", Error: interruptedBackupReason, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/backups/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "latest backup failed") {
		t.Fatalf("backup status omitted the failure warning: %d %s", response.Code, response.Body.String())
	}
}

func TestBackupCompletionEventRequiresDurableState(t *testing.T) {
	server := testServer(t)
	events, unsubscribe := server.hub.Subscribe()
	defer unsubscribe()
	if err := server.store.Close(); err != nil {
		t.Fatal(err)
	}

	server.finishBackup(backup.Run{ID: "not-durable", Actor: "admin"}, "verified", nil)
	select {
	case event := <-events:
		t.Fatalf("published backup event after state persistence failed: %#v", event)
	default:
	}
}

func TestBackupDoesNotStartWhenRunningStateCannotPersist(t *testing.T) {
	server := testServer(t)
	var logs bytes.Buffer
	server.log = slog.New(slog.NewJSONHandler(&logs, nil))
	if err := server.store.Close(); err != nil {
		t.Fatal(err)
	}

	server.executeBackup(backup.Run{ID: "blocked", Actor: "system", State: "queued", StartedAt: time.Now().UTC()})
	if !strings.Contains(logs.String(), `"msg":"backup state persistence failed"`) {
		t.Fatalf("backup start did not record the persistence failure: %s", logs.String())
	}
}

func TestBackupScheduleAPIRequiresValidIntervalAndPersists(t *testing.T) {
	server := testServer(t)
	invalid := httptest.NewRecorder()
	server.routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodPatch, "/api/v1/backups/schedule", strings.NewReader(`{"enabled":true,"intervalSeconds":30}`)))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected schedule validation, got %d: %s", invalid.Code, invalid.Body.String())
	}
	updated := httptest.NewRecorder()
	server.routes().ServeHTTP(updated, httptest.NewRequest(http.MethodPatch, "/api/v1/backups/schedule", strings.NewReader(`{"enabled":false,"intervalSeconds":7200}`)))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"intervalSeconds":7200`) {
		t.Fatalf("schedule update failed: %d: %s", updated.Code, updated.Body.String())
	}
	readback := httptest.NewRecorder()
	server.routes().ServeHTTP(readback, httptest.NewRequest(http.MethodGet, "/api/v1/backups/schedule", nil))
	if readback.Code != http.StatusOK || !strings.Contains(readback.Body.String(), `"enabled":false`) {
		t.Fatalf("schedule readback failed: %d: %s", readback.Code, readback.Body.String())
	}
}

func TestAutomaticBackupFailsWhenDockerAppdataExportIsIncomplete(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  media:\n    image: example/media:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(catalog, []byte(`[{"id":"media","image":"example/media:latest","recovery":{"strategy":"stop-backup","appdataPaths":["/config"]}}]`), 0o640); err != nil {
		t.Fatal(err)
	}
	recoveryDir := t.TempDir()
	t.Setenv("LUMONAS_RECOVERY_KEY", "backup-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", recoveryDir)
	server := testServer(t)
	t.Setenv("LUMONAS_AUTO_BACKUP_DISABLED", "false")
	server.catalogFile = catalog
	server.dockerService = dockerruntime.New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		if strings.Contains(command, "config --format json") {
			return []byte(`{"services":{"media":{"volumes":[{"type":"bind","source":"/srv/lumonas/missing-appdata","target":"/config"}]}}}`), nil
		}
		return nil, nil
	})
	run := backup.Run{ID: "run-incomplete", Trigger: "scheduled", State: "queued", StartedAt: time.Now().UTC()}
	if err := server.store.SaveBackupRun(run); err != nil {
		t.Fatal(err)
	}
	server.executeBackup(run)
	runs, err := server.store.BackupRuns(1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("backup run was not persisted: %#v %v", runs, err)
	}
	if runs[0].State != "failed" || !strings.Contains(runs[0].Error, "incomplete") {
		t.Fatalf("incomplete appdata backup was marked successful: %#v", runs[0])
	}
}
