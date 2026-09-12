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

func TestProtectionConfigEndpointReportsUnconfiguredState(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/storage/protection/config", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"configured":false`) {
		t.Fatalf("expected unconfigured protection, got %d: %s", response.Code, response.Body.String())
	}
}

func TestProtectionConfigUpdateValidatesDisksAndBroker(t *testing.T) {
	server := testServer(t)
	unknown := httptest.NewRecorder()
	server.routes().ServeHTTP(unknown, httptest.NewRequest(http.MethodPut, "/api/v1/storage/protection/config", strings.NewReader(`{"parityDiskId":"wwn:ghost","dataDiskIds":["wwn:test"]}`)))
	if unknown.Code != http.StatusUnprocessableEntity || !strings.Contains(unknown.Body.String(), "wwn:ghost") {
		t.Fatalf("expected unknown disk rejection, got %d: %s", unknown.Code, unknown.Body.String())
	}
	noData := httptest.NewRecorder()
	server.routes().ServeHTTP(noData, httptest.NewRequest(http.MethodPut, "/api/v1/storage/protection/config", strings.NewReader(`{"parityDiskId":"wwn:test","dataDiskIds":[]}`)))
	if noData.Code != http.StatusUnprocessableEntity || !strings.Contains(noData.Body.String(), "at least one") {
		t.Fatalf("expected data disk requirement, got %d: %s", noData.Code, noData.Body.String())
	}
	overlap := httptest.NewRecorder()
	server.routes().ServeHTTP(overlap, httptest.NewRequest(http.MethodPut, "/api/v1/storage/protection/config", strings.NewReader(`{"parityDiskId":"wwn:test","dataDiskIds":["wwn:test"]}`)))
	if overlap.Code != http.StatusUnprocessableEntity || !strings.Contains(overlap.Body.String(), "cannot also be") {
		t.Fatalf("expected parity overlap rejection, got %d: %s", overlap.Code, overlap.Body.String())
	}
	// With a valid layout the request proceeds to the broker, which is
	// unavailable in tests, so the API reports the broker dependency.
	broker := httptest.NewRecorder()
	server.routes().ServeHTTP(broker, httptest.NewRequest(http.MethodPut, "/api/v1/storage/protection/config", strings.NewReader(`{"parityDiskId":"wwn:test","dataDiskIds":["wwn:test"]}`)))
	_ = broker
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{
			{ID: "wwn:test", Serial: "one", SizeBytes: 100, Health: model.Healthy},
			{ID: "wwn:second", Serial: "two", SizeBytes: 200, Health: model.Healthy},
		}, nil
	}
	broker = httptest.NewRecorder()
	server.routes().ServeHTTP(broker, httptest.NewRequest(http.MethodPut, "/api/v1/storage/protection/config", strings.NewReader(`{"parityDiskId":"wwn:test","dataDiskIds":["wwn:second"]}`)))
	if broker.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected broker unavailability, got %d: %s", broker.Code, broker.Body.String())
	}
}

func TestOnboardingAppliesProtectionLayoutAndSchedules(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{
			{ID: "serial:data", Serial: "data", SizeBytes: 200, Health: model.Healthy},
			{ID: "serial:second", Serial: "second", SizeBytes: 210, Health: model.Healthy},
			{ID: "serial:parity", Serial: "parity", SizeBytes: 300, Health: model.Healthy},
		}, nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/complete", strings.NewReader(`{"serverName":"nas-lab","roles":{"serial:data":"data","serial:second":"data","serial:parity":"parity"},"protection":{"syncTime":"01:30","scrubDay":"friday"},"recovery":{"autoConfigBackup":false,"destination":"","keyAcknowledged":false}}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("onboarding completion failed: %d %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["protectionConfigured"] != false || result["initialSyncStarted"] != false {
		t.Fatalf("onboarding must not start protection work after broker failure: %#v", result)
	}
	schedules, err := server.store.JobSchedules(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	syncTime := ""
	scrubDay := ""
	for _, schedule := range schedules {
		switch schedule.ID {
		case "sched-sync":
			syncTime = schedule.TimeOfDay
		case "sched-scrub":
			scrubDay = schedule.Weekday
		}
	}
	if syncTime != "01:30" || scrubDay != "friday" {
		t.Fatalf("onboarding schedules were not applied: sync=%q scrub=%q", syncTime, scrubDay)
	}
	parity, data := onboardingProtectionDisks(map[string]string{"serial:data": "data", "serial:second": "data", "serial:parity": "parity"})
	if parity != "serial:parity" || len(data) != 2 {
		t.Fatalf("unexpected protection layout: parity=%q data=%v", parity, data)
	}
}
