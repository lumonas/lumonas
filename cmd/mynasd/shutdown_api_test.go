package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShutdownPlanIsExplicitAndRequiresReauthentication(t *testing.T) {
	server := testServer(t)
	plan := httptest.NewRecorder()
	server.routes().ServeHTTP(plan, httptest.NewRequest(http.MethodGet, "/api/v1/power/shutdown/plan?action=poweroff", nil))
	if plan.Code != http.StatusOK || !strings.Contains(plan.Body.String(), `"name":"stop-jobs"`) || !strings.Contains(plan.Body.String(), `"name":"unmount-storage"`) {
		t.Fatalf("unexpected shutdown plan: %d %s", plan.Code, plan.Body.String())
	}

	rejected := httptest.NewRecorder()
	server.routes().ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/api/v1/power/shutdown", strings.NewReader(`{"action":"poweroff"}`)))
	if rejected.Code != http.StatusLocked {
		t.Fatalf("expected reauthentication lock, got %d: %s", rejected.Code, rejected.Body.String())
	}

	invalid := httptest.NewRecorder()
	server.routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/power/shutdown/plan?action=destroy", nil))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid action rejection, got %d", invalid.Code)
	}
}
