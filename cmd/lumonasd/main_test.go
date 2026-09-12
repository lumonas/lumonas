package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
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
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/storage"
	"github.com/lumonas/lumonas/internal/store"
	"github.com/lumonas/lumonas/internal/trace"
)

func testServer(t *testing.T) *apiServer {
	t.Helper()
	// API tests should not launch asynchronous production backups after the
	// store cleanup has started; backup scheduling has dedicated tests.
	t.Setenv("LUMONAS_AUTO_BACKUP_DISABLED", "true")
	// API tests do not configure notification channels. Suppress the
	// asynchronous warning delivery path so event publishing cannot outlive
	// the SQLite store cleanup.
	t.Setenv("LUMONAS_NOTIFY_MIN_SEVERITY", "off")
	db, err := store.Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.SetMeta("nas_uuid", "nas-test"); err != nil {
		t.Fatal(err)
	}
	return &apiServer{store: db, hub: events.NewHub(), version: "test", diskFunc: func() ([]model.Disk, error) {
		return []model.Disk{{ID: "wwn:test", Name: "sda", Role: "unknown", Health: model.Healthy, LastSeen: time.Now().UTC()}}, nil
	}, brokerExec: func(context.Context, privileged.Request) error { return nil }, csrfTokens: make(map[string]csrfBinding), rateAttempts: make(map[string][]time.Time)}
}

func TestRequestMiddlewarePropagatesCorrelationID(t *testing.T) {
	var got string
	server := &apiServer{}
	handler := server.requestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = trace.CorrelationID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusNoContent || got == "" {
		t.Fatalf("request correlation was not propagated: status=%d id=%q", response.Code, got)
	}
	if response.Header().Get("X-Request-ID") != got {
		t.Fatalf("response correlation header %q does not match context %q", response.Header().Get("X-Request-ID"), got)
	}
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

func TestStorageCreatePlanValidatesRequestedStateAndSafety(t *testing.T) {
	server := testServer(t)
	invalid := httptest.NewRequest(http.MethodPost, "/api/v1/storage/operations/plan", strings.NewReader(`{"action":"filesystem.create","diskId":"wwn:test","requestedState":{"filesystem":"ext4","mountPath":"/mnt/other"}}`))
	invalidResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidResponse.Body.String(), "canonical") {
		t.Fatalf("expected 422 canonical path rejection, got %d: %s", invalidResponse.Code, invalidResponse.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storage/operations/plan", strings.NewReader(`{"action":"filesystem.create","diskId":"wwn:test","requestedState":{"filesystem":"ext4","mountPath":"/srv/disks/wwn_test","label":"media"}}`))
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
	confirm := httptest.NewRequest(http.MethodPost, "/api/v1/storage/operations/"+plan.OperationID+"/confirm", strings.NewReader(`{"planHash":"`+plan.PlanHash+`","reauthenticated":true}`))
	confirmed := httptest.NewRecorder()
	server.routes().ServeHTTP(confirmed, confirm)
	if confirmed.Code != http.StatusLocked {
		t.Fatalf("expected safety lock on confirm, got %d", confirmed.Code)
	}
}

func TestStorageMountsEndpointReturnsPersistedState(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/mounts", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"entries":[]`) {
		t.Fatalf("expected empty mount state, got %d: %s", response.Code, response.Body.String())
	}
	entries := []storage.MountEntry{{Kind: "disk", TargetID: "wwn:test", MountPath: "/srv/disks/wwn_test", FSType: "ext4", Source: "UUID=abc", Options: storage.DiskMountOptions, Enabled: true}}
	if err := server.store.SaveMountEntries(entries); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/mounts", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mountPath":"/srv/disks/wwn_test"`) {
		t.Fatalf("expected persisted entry, got %d: %s", response.Code, response.Body.String())
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

func TestDockerStacksExposeCatalogRecoveryMetadata(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "jellyfin")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  jellyfin:\n    image: jellyfin/jellyfin:10.10.6\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(catalogPath, []byte(`[{"id":"jellyfin","category":"Media","image":"jellyfin/jellyfin:10.10.6","appdataPaths":["/config"],"recovery":{"strategy":"stop-backup","appdataPaths":["/config"]}}]`), 0o640); err != nil {
		t.Fatal(err)
	}
	server := testServer(t)
	server.catalogFile = catalogPath
	server.dockerService = dockerruntime.New(root, func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil })
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/docker/stacks", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var stacks []dockerruntime.Stack
	if err := json.NewDecoder(response.Body).Decode(&stacks); err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 1 || stacks[0].CatalogID != "jellyfin" || stacks[0].Recovery == nil || stacks[0].RecoveryCoverage <= 0 {
		t.Fatalf("unexpected catalog recovery metadata: %#v", stacks)
	}
}

func TestDockerImageImportStagesBoundedArchiveAndLoadsIt(t *testing.T) {
	server := testServer(t)
	importRoot := t.TempDir()
	t.Setenv("LUMONAS_DOCKER_IMPORT_DIR", importRoot)
	var command string
	server.dockerService = dockerruntime.New(t.TempDir(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	body := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(body)
	part, err := multipartWriter.CreateFormFile("archive", "images.tar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("docker save payload")); err != nil {
		t.Fatal(err)
	}
	if err := multipartWriter.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/docker/images/import", body)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"imported"`) {
		t.Fatalf("unexpected import response %d: %s", response.Code, response.Body.String())
	}
	if !strings.HasPrefix(command, "docker load --input "+importRoot) {
		t.Fatalf("unexpected import command: %q", command)
	}
}

func TestDockerMutationsRequireManagementIdentity(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/docker/stacks", strings.NewReader(`{"name":"media","composeYaml":"services:\n  media:\n    image: example/media:latest\n"}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected Docker mutation to require management identity, got %d: %s", response.Code, response.Body.String())
	}
}

func TestRecoveryPlanWarnsOnNASAndDiskMismatch(t *testing.T) {
	server := testServer(t)
	directory := t.TempDir()
	t.Setenv("LUMONAS_RECOVERY_KEY", "test-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", directory)
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
	t.Setenv("LUMONAS_RECOVERY_KEY", "stage-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", directory)
	staging := filepath.Join(directory, "staged")
	t.Setenv("LUMONAS_RECOVERY_STAGING_DIR", staging)
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
