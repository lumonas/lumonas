package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestAlertHistoryRecordsResolvedAlerts(t *testing.T) {
	server := testServer(t)

	// Fire a rule alert against a disk, then resolve it by restoring health.
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{smartTestDisk("wwan:worn", model.SmartSummary{Overall: model.Warning, PendingSectors: 9})}, nil
	}
	server.evaluateSMARTAlerts()
	live, err := server.store.GeneratedAlerts()
	if err != nil || len(live) != 1 {
		t.Fatalf("expected one firing alert, got %#v err=%v", live, err)
	}

	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{smartTestDisk("wwan:worn", model.SmartSummary{Overall: model.Healthy})}, nil
	}
	server.evaluateSMARTAlerts()
	live, _ = server.store.GeneratedAlerts()
	if len(live) != 0 {
		t.Fatalf("alert did not resolve: %#v", live)
	}

	// The active alerts endpoint must not include the resolved alert.
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("alerts endpoint failed: %d", response.Code)
	}
	var active []model.Alert
	if err := json.NewDecoder(response.Body).Decode(&active); err != nil {
		t.Fatal(err)
	}
	for _, alert := range active {
		if alert.State == "resolved" {
			t.Fatalf("resolved alert leaked into active list: %#v", alert)
		}
	}

	// History shows the resolved alert with its resolution timestamp.
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/alerts/history", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("history endpoint failed: %d", response.Code)
	}
	var history []model.Alert
	if err := json.NewDecoder(response.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one history entry, got %#v", history)
	}
	if history[0].State != "resolved" || history[0].ResolvedAt == nil {
		t.Fatalf("history entry incomplete: %#v", history[0])
	}
	if history[0].Resource == nil || history[0].Resource.ID != "wwan:worn" {
		t.Fatalf("history entry lost its resource: %#v", history[0])
	}

	// Limit validation.
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/alerts/history?limit=zero", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad limit, got %d", response.Code)
	}
}
