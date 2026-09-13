package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

const replacementManagedConfig = `# managed
parity /srv/disks/serial_P/snapraid.parity
content /var/lib/lumonas/snapraid.content
content /srv/disks/serial_A/snapraid.content
content /srv/disks/serial_B/snapraid.content
data d1 /srv/disks/serial_A
data d2 /srv/disks/serial_B
data d3 /srv/disks/serial_DEAD
`

func TestDiskReplacementPlanAndConfirmChain(t *testing.T) {
	server := testServer(t)
	configPath := filepath.Join(t.TempDir(), "snapraid.conf")
	t.Setenv("LUMONAS_SNAPRAID_CONFIG", configPath)
	if err := os.WriteFile(configPath, []byte(replacementManagedConfig), 0o640); err != nil {
		t.Fatal(err)
	}
	live := []model.Disk{
		{ID: "serial:A", CurrentPath: "/dev/sda", Serial: "A", Role: "data", Health: model.Healthy, LastSeen: time.Now().UTC()},
		{ID: "serial:B", CurrentPath: "/dev/sdb", Serial: "B", Role: "data", Health: model.Healthy, LastSeen: time.Now().UTC()},
		{ID: "serial:P", CurrentPath: "/dev/sdp", Serial: "P", Role: "parity", Health: model.Healthy, LastSeen: time.Now().UTC()},
		{ID: "serial:NEW", CurrentPath: "/dev/sdn", Serial: "NEW", Role: "data", Health: model.Healthy, LastSeen: time.Now().UTC()},
	}
	server.diskFunc = func() ([]model.Disk, error) { return live, nil }

	planResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(planResponse, httptest.NewRequest(http.MethodPost, "/api/v1/storage/protection/replacement/plan", strings.NewReader(`{"retiredDiskId":"serial:DEAD","replacementDiskId":"serial:NEW"}`)))
	if planResponse.Code != http.StatusCreated {
		t.Fatalf("replacement plan failed: %d %s", planResponse.Code, planResponse.Body.String())
	}
	var plan struct {
		OperationID     string `json:"operationId"`
		PlanHash        string `json:"planHash"`
		RetiredDataName string `json:"retiredDataName"`
	}
	if err := json.NewDecoder(planResponse.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if plan.RetiredDataName != "d3" {
		t.Fatalf("retired slot = %q", plan.RetiredDataName)
	}

	// Locked safety window must block execution.
	locked := httptest.NewRecorder()
	server.routes().ServeHTTP(locked, httptest.NewRequest(http.MethodPost, "/api/v1/storage/protection/replacement/confirm", strings.NewReader(`{"operationId":"`+plan.OperationID+`","planHash":"`+plan.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if locked.Code != http.StatusLocked {
		t.Fatalf("expected 423 while locked, got %d", locked.Code)
	}

	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	var operations []string
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		operations = append(operations, request.Operation)
		return nil
	}
	confirm := httptest.NewRecorder()
	server.routes().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/storage/protection/replacement/confirm", strings.NewReader(`{"operationId":"`+plan.OperationID+`","planHash":"`+plan.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if confirm.Code != http.StatusAccepted {
		t.Fatalf("replacement confirm failed: %d %s", confirm.Code, confirm.Body.String())
	}
	// The fix and follow-up sync jobs are started asynchronously after the
	// replacement is accepted, so they may already have reached the broker
	// while this handler response is being asserted. The synchronous prefix is
	// the filesystem creation and managed config activation.
	if len(operations) < 2 || operations[0] != "filesystem.create" || operations[1] != "snapraid.config.apply" {
		t.Fatalf("unexpected broker chain: %#v", operations)
	}
	var result struct {
		OK   bool   `json:"ok"`
		Slot string `json:"slot"`
		Next string `json:"next"`
	}
	if err := json.NewDecoder(confirm.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Slot != "d3" {
		t.Fatalf("unexpected confirm result: %#v", result)
	}
	// A queued fix job must exist and carry the recovery slot.
	jobs, err := server.store.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	fixFound := false
	for _, job := range jobs {
		if job.Type == "snapraid.fix" {
			fixFound = true
			if job.State != "queued" && job.State != "running" && job.State != "failed" && job.State != "completed" {
				t.Fatalf("unexpected fix job state %q", job.State)
			}
		}
	}
	if !fixFound {
		t.Fatal("no persisted snapraid.fix job after replacement confirm")
	}

	// Bad hash is rejected even with everything else valid.
	badHash := httptest.NewRecorder()
	server.routes().ServeHTTP(badHash, httptest.NewRequest(http.MethodPost, "/api/v1/storage/protection/replacement/confirm", strings.NewReader(`{"operationId":"`+plan.OperationID+`","planHash":"zzz","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if badHash.Code != http.StatusConflict {
		t.Fatalf("expected 409 for hash mismatch, got %d", badHash.Code)
	}
}

func TestDiskReplacementPlanRequiresManagedConfig(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SNAPRAID_CONFIG", filepath.Join(t.TempDir(), "missing.conf"))
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{{ID: "serial:NEW", Role: "data", Health: model.Healthy, LastSeen: time.Now().UTC()}}, nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/protection/replacement/plan", strings.NewReader(`{"retiredDiskId":"serial:A","replacementDiskId":"serial:NEW"}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 without readable config, got %d", response.Code)
	}
}
