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
)

func setupDisk(id string, filesystem string, role string) model.Disk {
	return model.Disk{ID: id, CurrentPath: "/dev/sd" + id, WWN: "wwn-" + id, Serial: "serial-" + id, Model: "TestDisk", SizeBytes: 100, Filesystem: filesystem, Role: role, Health: model.Healthy, LastSeen: time.Now().UTC()}
}

func TestPoolSetupPlanListsFormatStepsAndSurvivesValidation(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{
			setupDisk("a", "", "data"),
			setupDisk("b", "ext4", "data"),
			setupDisk("p", "", "parity"),
		}, nil
	}

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/plan", strings.NewReader(`{"name":"media","dataDiskIds":["a","b"],"parityDiskId":"p"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("setup plan failed: %d %s", response.Code, response.Body.String())
	}
	var plan struct {
		OperationID   string   `json:"operationId"`
		PlanHash      string   `json:"planHash"`
		MountPath     string   `json:"mountPath"`
		FormatDiskIDs []string `json:"formatDiskIds"`
		MountDiskIDs  []string `json:"mountDiskIds"`
		DestroysData  bool     `json:"destroysData"`
		Steps         []struct {
			Action string `json:"action"`
		} `json:"steps"`
	}
	if err := json.NewDecoder(response.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if plan.MountPath != "/srv/pools/media" {
		t.Fatalf("unexpected mount path %q", plan.MountPath)
	}
	// Disk "a" and blank parity "p" are formatted; existing "b" is mounted.
	if len(plan.FormatDiskIDs) != 2 || plan.FormatDiskIDs[0] != "a" || plan.FormatDiskIDs[1] != "p" {
		t.Fatalf("unexpected format list: %#v", plan.FormatDiskIDs)
	}
	if len(plan.MountDiskIDs) != 1 || plan.MountDiskIDs[0] != "b" {
		t.Fatalf("unexpected mount list: %#v", plan.MountDiskIDs)
	}
	if !plan.DestroysData {
		t.Fatal("plan with formatting must flag destroysData")
	}
	actions := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		actions = append(actions, step.Action)
	}
	if !strings.Contains(strings.Join(actions, ","), "filesystem.create") || !strings.Contains(strings.Join(actions, ","), "pool.mount") || !strings.Contains(strings.Join(actions, ","), "snapraid.apply") {
		t.Fatalf("unexpected step sequence: %#v", actions)
	}

	// Re-planning with every disk already formatted marks nothing destructive.
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{setupDisk("b", "ext4", "data")}, nil
	}
	clean := httptest.NewRecorder()
	server.routes().ServeHTTP(clean, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/plan", strings.NewReader(`{"name":"media","dataDiskIds":["b"]}`)))
	if clean.Code != http.StatusCreated {
		t.Fatalf("clean setup plan failed: %d", clean.Code)
	}
	var cleanPlan struct {
		DestroysData  bool     `json:"destroysData"`
		FormatDiskIDs []string `json:"formatDiskIds"`
	}
	if err := json.NewDecoder(clean.Body).Decode(&cleanPlan); err != nil {
		t.Fatal(err)
	}
	if cleanPlan.DestroysData || len(cleanPlan.FormatDiskIDs) != 0 {
		t.Fatalf("pre-formatted disk should not require formatting: %#v", cleanPlan)
	}
}

func TestPoolSetupPlanRejectsBadLayouts(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{setupDisk("a", "ext4", "data"), setupDisk("p", "", "parity")}, nil
	}
	cases := []struct {
		name   string
		body   string
		expect string
	}{
		{"empty data", `{"name":"media","dataDiskIds":[]}`, "at least one data disk"},
		{"parity as data", `{"name":"media","dataDiskIds":["p"]}`, "cannot become pool data"},
		{"same disk both", `{"name":"media","dataDiskIds":["a"],"parityDiskId":"a"}`, "both parity and pool data"},
		{"bad name", `{"name":"Big Pool","dataDiskIds":["a"]}`, "pool name"},
		{"unknown parity", `{"name":"media","dataDiskIds":["a"],"parityDiskId":"zz"}`, "not currently discovered"},
	}
	for _, testCase := range cases {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/plan", strings.NewReader(testCase.body)))
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), testCase.expect) {
			t.Fatalf("%s: expected 422 with %q, got %d: %s", testCase.name, testCase.expect, response.Code, response.Body.String())
		}
	}
}

func TestPoolSetupConfirmRequiresSafetyAndHash(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{setupDisk("a", "ext4", "data")}, nil
	}
	plan := httptest.NewRecorder()
	server.routes().ServeHTTP(plan, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/plan", strings.NewReader(`{"name":"media","dataDiskIds":["a"]}`)))
	var parsed struct {
		OperationID string `json:"operationId"`
		PlanHash    string `json:"planHash"`
	}
	if err := json.NewDecoder(plan.Body).Decode(&parsed); err != nil {
		t.Fatal(err)
	}

	// Locked safety window rejects execution even with a valid hash.
	confirm := httptest.NewRecorder()
	server.routes().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/confirm", strings.NewReader(`{"operationId":"`+parsed.OperationID+`","planHash":"`+parsed.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if confirm.Code != http.StatusLocked {
		t.Fatalf("expected 423 while safety locked, got %d", confirm.Code)
	}

	// Wrong hash with the window open still fails.
	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	badHash := httptest.NewRecorder()
	server.routes().ServeHTTP(badHash, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/confirm", strings.NewReader(`{"operationId":"`+parsed.OperationID+`","planHash":"deadbeef","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if badHash.Code != http.StatusConflict {
		t.Fatalf("expected 409 for hash mismatch, got %d", badHash.Code)
	}

	// Unknown operation id is a 404.
	missing := httptest.NewRecorder()
	server.routes().ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/confirm", strings.NewReader(`{"operationId":"pool-setup-nope","planHash":"x","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown plan, got %d", missing.Code)
	}
}

func TestPoolSetupConfirmExecutesChain(t *testing.T) {
	server := testServer(t)
	disk := setupDisk("a", "", "data")
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{disk}, nil }

	var executed []string
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		executed = append(executed, request.Operation)
		return nil
	}

	plan := httptest.NewRecorder()
	server.routes().ServeHTTP(plan, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/plan", strings.NewReader(`{"name":"media","dataDiskIds":["a"]}`)))
	var parsed struct {
		OperationID string `json:"operationId"`
		PlanHash    string `json:"planHash"`
	}
	if err := json.NewDecoder(plan.Body).Decode(&parsed); err != nil {
		t.Fatal(err)
	}

	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	// After formatting the disk reports an ext4 filesystem and a fresh UUID.
	disk.Filesystem = "ext4"
	disk.FilesystemUUID = "new-fs-uuid"
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{disk}, nil }

	confirm := httptest.NewRecorder()
	server.routes().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/confirm", strings.NewReader(`{"operationId":"`+parsed.OperationID+`","planHash":"`+parsed.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if confirm.Code != http.StatusAccepted {
		t.Fatalf("setup confirm failed: %d %s", confirm.Code, confirm.Body.String())
	}
	if len(executed) != 2 || executed[0] != "filesystem.create" || executed[1] != "pool.mount" {
		t.Fatalf("unexpected execution chain: %#v", executed)
	}
	var result struct {
		OK        bool   `json:"ok"`
		MountPath string `json:"mountPath"`
	}
	if err := json.NewDecoder(confirm.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.MountPath != "/srv/pools/media" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestPoolSetupConfirmMountsExistingFilesystemBeforePool(t *testing.T) {
	server := testServer(t)
	disk := setupDisk("a", "ext4", "data")
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{disk}, nil }
	executed := make([]string, 0, 2)
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		executed = append(executed, request.Operation)
		return nil
	}
	plan := httptest.NewRecorder()
	server.routes().ServeHTTP(plan, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/plan", strings.NewReader(`{"name":"media","dataDiskIds":["a"]}`)))
	if plan.Code != http.StatusCreated {
		t.Fatalf("setup plan failed: %d %s", plan.Code, plan.Body.String())
	}
	var parsed struct {
		OperationID string `json:"operationId"`
		PlanHash    string `json:"planHash"`
	}
	if err := json.NewDecoder(plan.Body).Decode(&parsed); err != nil {
		t.Fatal(err)
	}
	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	confirm := httptest.NewRecorder()
	server.routes().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/setup/confirm", strings.NewReader(`{"operationId":"`+parsed.OperationID+`","planHash":"`+parsed.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if confirm.Code != http.StatusAccepted {
		t.Fatalf("setup confirm failed: %d %s", confirm.Code, confirm.Body.String())
	}
	if len(executed) != 2 || executed[0] != "filesystem.mount" || executed[1] != "pool.mount" {
		t.Fatalf("existing filesystem was not mounted before pool: %#v", executed)
	}
}
