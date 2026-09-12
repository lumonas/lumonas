package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMonitoringConfigurationEndpointsPersistAlertRuleChanges(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/alert-rules", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected rules, got %d: %s", response.Code, response.Body.String())
	}
	var rules []struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(response.Body).Decode(&rules); err != nil || len(rules) == 0 {
		t.Fatalf("unexpected rules: %#v err=%v", rules, err)
	}
	first := rules[0]
	update := httptest.NewRecorder()
	server.routes().ServeHTTP(update, httptest.NewRequest(http.MethodPatch, "/api/v1/alert-rules/"+first.ID, strings.NewReader(`{"enabled":false}`)))
	if update.Code != http.StatusOK || strings.Contains(update.Body.String(), `"enabled":true`) {
		t.Fatalf("unexpected rule update %d: %s", update.Code, update.Body.String())
	}
	channels := httptest.NewRecorder()
	server.routes().ServeHTTP(channels, httptest.NewRequest(http.MethodGet, "/api/v1/notification-channels", nil))
	if channels.Code != http.StatusOK || !strings.Contains(channels.Body.String(), `"type":"web"`) {
		t.Fatalf("unexpected notification channels %d: %s", channels.Code, channels.Body.String())
	}
	schedules := httptest.NewRecorder()
	server.routes().ServeHTTP(schedules, httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil))
	if schedules.Code != http.StatusOK || !strings.Contains(schedules.Body.String(), "SnapRAID sync") {
		t.Fatalf("unexpected schedules %d: %s", schedules.Code, schedules.Body.String())
	}
}
