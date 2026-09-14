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

func TestPoolMembershipPlanRejectsUnknownPool(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{}, nil }
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/membership/plan", strings.NewReader(`{"pool":"media","addDiskIds":["serial:A"]}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unmounted pool, got %d", response.Code)
	}
}

func TestPoolMembershipPlanAndConfirmChain(t *testing.T) {
	server := testServer(t)
	configPath := filepath.Join(t.TempDir(), "snapraid.conf")
	t.Setenv("LUMONAS_SNAPRAID_CONFIG", configPath)
	if err := os.WriteFile(configPath, []byte(`# managed
parity /srv/disks/serial_P/snapraid.parity
content /var/lib/lumonas/snapraid.content
content /srv/disks/serial_A/snapraid.content
data d1 /srv/disks/serial_A
data d2 /srv/disks/serial_B
`), 0o640); err != nil {
		t.Fatal(err)
	}
	disks := []model.Disk{
		{ID: "serial:A", CurrentPath: "/dev/sda", Role: "data", Filesystem: "ext4", Health: model.Healthy, LastSeen: time.Now().UTC()},
		{ID: "serial:B", CurrentPath: "/dev/sdb", Role: "data", Filesystem: "ext4", Health: model.Healthy, LastSeen: time.Now().UTC()},
		{ID: "serial:P", CurrentPath: "/dev/sdp", Role: "parity", Health: model.Healthy, LastSeen: time.Now().UTC()},
		{ID: "serial:C", CurrentPath: "/dev/sdc", Role: "data", Health: model.Healthy, LastSeen: time.Now().UTC()},
	}
	server.diskFunc = func() ([]model.Disk, error) { return disks, nil }
	// The live discovery sees the pool mounted over A+B.
	server.discoverPoolsFunc = func(current []model.Disk) []model.Pool {
		_ = current
		return []model.Pool{{
			ID: "pool-1", Name: "media", MountPath: "/srv/pools/media", Type: "mergerfs",
			Members: []model.PoolMember{
				{Enabled: true, BranchPath: "/srv/disks/serial_A"},
				{Enabled: true, BranchPath: "/srv/disks/serial_B"},
			},
		}}
	}

	planResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(planResponse, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/membership/plan", strings.NewReader(`{"pool":"media","addDiskIds":["serial:C"]}`)))
	if planResponse.Code != http.StatusCreated {
		t.Fatalf("membership plan failed: %d %s", planResponse.Code, planResponse.Body.String())
	}
	var plan struct {
		OperationID   string   `json:"operationId"`
		PlanHash      string   `json:"planHash"`
		FormatDiskIDs []string `json:"formatDiskIds"`
		NewBranches   []string `json:"newBranches"`
	}
	if err := json.NewDecoder(planResponse.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.FormatDiskIDs) != 1 || plan.FormatDiskIDs[0] != "serial:C" {
		t.Fatalf("blank disk not queued for formatting: %#v", plan.FormatDiskIDs)
	}
	if len(plan.NewBranches) != 3 {
		t.Fatalf("unexpected branches: %#v", plan.NewBranches)
	}

	// Safety locked blocks execution.
	locked := httptest.NewRecorder()
	server.routes().ServeHTTP(locked, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/membership/confirm", strings.NewReader(`{"operationId":"`+plan.OperationID+`","planHash":"`+plan.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if locked.Code != http.StatusLocked {
		t.Fatalf("expected 423 while locked, got %d", locked.Code)
	}

	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	var operations []string
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		operations = append(operations, request.Operation)
		return nil
	}
	// After formatting, C reports ext4.
	disks[3].Filesystem = "ext4"
	server.diskFunc = func() ([]model.Disk, error) { return disks, nil }

	confirm := httptest.NewRecorder()
	server.routes().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/membership/confirm", strings.NewReader(`{"operationId":"`+plan.OperationID+`","planHash":"`+plan.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if confirm.Code != http.StatusAccepted {
		t.Fatalf("membership confirm failed: %d %s", confirm.Code, confirm.Body.String())
	}
	var result struct {
		OK                bool     `json:"ok"`
		Added             []string `json:"added"`
		Members           int      `json:"members"`
		ProtectionUpdated bool     `json:"protectionUpdated"`
	}
	if err := json.NewDecoder(confirm.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || len(result.Added) != 1 || result.Members != 3 || !result.ProtectionUpdated {
		t.Fatalf("unexpected confirm result: %#v", result)
	}
	// Expected chain: format C, unmount pool, mount pool, extend snapraid config.
	if len(operations) != 4 || operations[0] != "filesystem.create" || operations[1] != "pool.unmount" || operations[2] != "pool.mount" || operations[3] != "snapraid.config.apply" {
		t.Fatalf("unexpected broker chain: %#v", operations)
	}
	// A sync job was queued for the extended parity set.
	jobs, err := server.store.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	syncQueued := false
	for _, job := range jobs {
		if job.Type == "snapraid.sync" {
			syncQueued = true
		}
	}
	if !syncQueued {
		t.Fatal("no snapraid.sync job queued after membership grow")
	}
}
