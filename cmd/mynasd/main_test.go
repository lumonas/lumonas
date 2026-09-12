package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/events"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/store"
)

func testServer(t *testing.T) *apiServer {
	t.Helper()
	// API tests should not launch asynchronous production backups after the
	// store cleanup has started; backup scheduling has dedicated tests.
	t.Setenv("MYNAS_AUTO_BACKUP_DISABLED", "true")
	// API tests do not configure notification channels. Suppress the
	// asynchronous warning delivery path so event publishing cannot outlive
	// the SQLite store cleanup.
	t.Setenv("MYNAS_NOTIFY_MIN_SEVERITY", "critical")
	db, err := store.Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.SetMeta("nas_uuid", "nas-test"); err != nil {
		t.Fatal(err)
	}
	return &apiServer{store: db, hub: events.NewHub(), version: "test", acknowledged: make(map[string]bool), diskFunc: func() ([]model.Disk, error) {
		return []model.Disk{{ID: "wwn:test", Name: "sda", Role: "unknown", Health: model.Healthy, LastSeen: time.Now().UTC()}}, nil
	}}
}

func TestAPIHealthAndDiskIdentity(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	var disks []model.Disk
	if err := json.NewDecoder(response.Body).Decode(&disks); err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 || disks[0].ID != "wwn:test" {
		t.Fatalf("unexpected disks %#v", disks)
	}
}

func TestSnapraidJobIsQueuedAndFailsThroughUnavailableBroker(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", io.NopCloser(strings.NewReader(`{"type":"snapraid.sync"}`)))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", response.Code)
	}
}

func TestStoragePlanRequiresStableIdentityAndSafetyUnlock(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storage/operations/plan", strings.NewReader(`{"action":"filesystem.format","diskId":"wwn:test"}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected plan creation, got %d: %s", response.Code, response.Body.String())
	}
	var plan struct {
		OperationID string `json:"operationId"`
		PlanHash    string `json:"planHash"`
	}
	if err := json.NewDecoder(response.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	confirm := httptest.NewRequest(http.MethodPost, "/api/v1/storage/operations/"+plan.OperationID+"/confirm", strings.NewReader(`{"planHash":"`+plan.PlanHash+`"}`))
	confirmed := httptest.NewRecorder()
	server.routes().ServeHTTP(confirmed, confirm)
	if confirmed.Code != http.StatusLocked {
		t.Fatalf("expected safety lock, got %d", confirmed.Code)
	}
}

func TestAuthRequiredProtectsAPIButNotHealth(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	apiResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/v1/server", nil))
	if apiResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected API auth, got %d", apiResponse.Code)
	}
	healthResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(healthResponse, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("expected health endpoint, got %d", healthResponse.Code)
	}
}

func TestStorageSafetyUnlockExpiresServerSide(t *testing.T) {
	server := testServer(t)
	unlock := httptest.NewRequest(http.MethodPost, "/api/v1/storage/safety/unlock", strings.NewReader(`{"reauthenticated":true}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, unlock)
	if response.Code != http.StatusOK {
		t.Fatalf("unlock status %d: %s", response.Code, response.Body.String())
	}
	status := httptest.NewRecorder()
	server.routes().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/storage/safety", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"state":"unlocked"`) {
		t.Fatalf("expected unlocked state: %d %s", status.Code, status.Body.String())
	}
	lock := httptest.NewRecorder()
	server.routes().ServeHTTP(lock, httptest.NewRequest(http.MethodPost, "/api/v1/storage/safety/lock", nil))
	if lock.Code != http.StatusOK {
		t.Fatalf("lock status %d", lock.Code)
	}
}

func TestWriteJSONOmitsBodyForNoContent(t *testing.T) {
	response := httptest.NewRecorder()
	writeJSON(response, http.StatusNoContent, map[string]string{"status": "sent"})
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("expected empty 204 body, got %q", response.Body.String())
	}
}

func TestDockerStacksReflectContainerHealth(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  media:\n    image: example/media:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := testServer(t)
	server.dockerService = dockerruntime.New(root, func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "docker" {
			return []byte(`{"ID":"container-1","Names":"media","Image":"example/media:latest","State":"Up 2 hours (unhealthy)","Ports":"","Labels":"com.docker.compose.project=media"}` + "\n"), nil
		}
		return nil, nil
	})
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/docker/stacks", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var stacks []struct {
		State  string `json:"state"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stacks); err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 1 || stacks[0].State != "unhealthy" || stacks[0].Status != "critical" {
		t.Fatalf("unexpected stack state %#v", stacks)
	}
}

func TestRecoveryPlanWarnsOnNASAndDiskMismatch(t *testing.T) {
	server := testServer(t)
	directory := t.TempDir()
	t.Setenv("MYNAS_RECOVERY_KEY", "test-key")
	t.Setenv("MYNAS_RECOVERY_DIR", directory)
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{NASUUID: "other-nas", DiskIDs: []string{"wwn:missing"}}, DesiredState: []byte("{}"), Database: []byte("sqlite")}, []byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "latest.mrb"), bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/recovery/plan", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var plan struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.NewDecoder(response.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(plan.Warnings, "\n")
	if !strings.Contains(body, "different NAS identity") || !strings.Contains(body, "wwn:missing") {
		t.Fatalf("expected identity warnings, got %#v", plan.Warnings)
	}
}

func TestRecoveryStageRequiresConfirmationAndExtractsBundle(t *testing.T) {
	server := testServer(t)
	directory := t.TempDir()
	t.Setenv("MYNAS_RECOVERY_KEY", "stage-key")
	t.Setenv("MYNAS_RECOVERY_DIR", directory)
	staging := filepath.Join(directory, "staged")
	t.Setenv("MYNAS_RECOVERY_STAGING_DIR", staging)
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{NASUUID: "nas-test"}, DesiredState: []byte(`{"mode":"safe"}`), Database: []byte("SQLite format 3\x00staged")}, []byte("stage-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "latest.mrb"), bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	locked := httptest.NewRecorder()
	server.routes().ServeHTTP(locked, httptest.NewRequest(http.MethodPost, "/api/v1/recovery/restore/stage", strings.NewReader(`{"confirmed":false,"reauthenticated":true}`)))
	if locked.Code != http.StatusLocked {
		t.Fatalf("expected confirmation lock, got %d", locked.Code)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/recovery/restore/stage", strings.NewReader(`{"confirmed":true,"reauthenticated":true}`)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Directory string `json:"directory"`
		Verified  bool   `json:"verified"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !strings.HasPrefix(result.Directory, staging) {
		t.Fatalf("unexpected stage response %#v", result)
	}
}
