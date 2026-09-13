package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestOnboardingStateClassifiesDisksWithoutMutatingThem(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{
			{ID: "serial:system", Model: "System", Serial: "system", SizeBytes: 100, Role: "system", Filesystem: "ext4", Health: model.Healthy, LastSeen: time.Now().UTC()},
			{ID: "serial:data", Model: "Data", Serial: "data", SizeBytes: 200, Health: model.Healthy, LastSeen: time.Now().UTC()},
			{ID: "serial:parity", Model: "Parity", Serial: "parity", SizeBytes: 300, Role: "parity", Filesystem: "ext4", Health: model.Healthy, LastSeen: time.Now().UTC()},
		}, nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/onboarding/state", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"classification":"system"`) || !strings.Contains(response.Body.String(), `"classification":"blank"`) || !strings.Contains(response.Body.String(), `"classification":"suspected-parity"`) {
		t.Fatalf("unexpected onboarding state: %d %s", response.Code, response.Body.String())
	}
}

func TestOnboardingProtectionWaitsForMountedDisks(t *testing.T) {
	roles := map[string]string{"serial:data": "data", "serial:parity": "parity"}
	disks := map[string]model.Disk{
		"serial:data":   {ID: "serial:data", Mounted: true},
		"serial:parity": {ID: "serial:parity", Mounted: true},
	}
	if !onboardingProtectionReady(roles, disks) {
		t.Fatal("mounted protection disks should be ready for the initial sync")
	}
	disks["serial:parity"] = model.Disk{ID: "serial:parity", Mounted: false}
	if onboardingProtectionReady(roles, disks) {
		t.Fatal("unmounted protection disks must defer the initial sync")
	}
}

func TestOnboardingProtectionRequiresCompleteLayout(t *testing.T) {
	disks := map[string]model.Disk{"serial:data": {ID: "serial:data", Mounted: true}}
	if onboardingProtectionReady(map[string]string{"serial:data": "data"}, disks) {
		t.Fatal("a protection layout requires both data and parity disks")
	}
}

func TestCompleteOnboardingPersistsSafeConfiguration(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{
			{ID: "serial:data", Serial: "data", SizeBytes: 200, Health: model.Healthy},
			{ID: "serial:apps", Serial: "apps", SizeBytes: 100, Health: model.Healthy},
		}, nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/complete", strings.NewReader(`{"serverName":"nas-lab","roles":{"serial:data":"data","serial:apps":"apps"},"protection":{"syncTime":"02:00","scrubDay":"sunday"},"recovery":{"autoConfigBackup":false,"destination":"","keyAcknowledged":false}}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":true`) {
		t.Fatalf("onboarding completion failed: %d %s", response.Code, response.Body.String())
	}
	if value, ok := server.store.Meta("server_name"); !ok || value != "nas-lab" {
		t.Fatalf("server name was not persisted: %q %v", value, ok)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["initialSyncStarted"] != false {
		t.Fatalf("unexpected initial sync state: %#v", result)
	}
}

func TestRecoveryKeyCanBeGeneratedDuringOnboarding(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_RECOVERY_DIR", t.TempDir())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/recovery/key", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"key"`) {
		t.Fatalf("recovery key generation failed: %d %s", response.Code, response.Body.String())
	}
	if key := server.recoveryKeyString(); len(key) != 64 {
		t.Fatalf("unexpected recovery key length: %d", len(key))
	}
}
