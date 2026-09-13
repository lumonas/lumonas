package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/privileged"
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

func TestUPSPolicyIsPersistedAndValidated(t *testing.T) {
	server := testServer(t)

	status := httptest.NewRecorder()
	server.routes().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/ups/status", nil))
	if status.Code != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(status.Body.String()), "[") {
		t.Fatalf("unexpected UPS status %d: %s", status.Code, status.Body.String())
	}

	invalid := httptest.NewRecorder()
	server.routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodPatch, "/api/v1/ups/policy", strings.NewReader(`{"enabled":true,"minimumRuntimeSec":-1,"minimumCharge":10}`)))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid UPS policy rejection, got %d: %s", invalid.Code, invalid.Body.String())
	}

	updated := httptest.NewRecorder()
	server.routes().ServeHTTP(updated, httptest.NewRequest(http.MethodPatch, "/api/v1/ups/policy", strings.NewReader(`{"enabled":true,"minimumRuntimeSec":120,"minimumCharge":15}`)))
	if updated.Code != http.StatusOK {
		t.Fatalf("expected UPS policy update, got %d: %s", updated.Code, updated.Body.String())
	}

	readback := httptest.NewRecorder()
	server.routes().ServeHTTP(readback, httptest.NewRequest(http.MethodGet, "/api/v1/ups/policy", nil))
	if readback.Code != http.StatusOK || !strings.Contains(readback.Body.String(), `"minimumRuntimeSec":120`) || !strings.Contains(readback.Body.String(), `"minimumCharge":15`) {
		t.Fatalf("unexpected persisted UPS policy %d: %s", readback.Code, readback.Body.String())
	}
}

func TestPowerActionRequiresAdministrativeIdentity(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/power/action", strings.NewReader(`{"action":"reboot","reauthenticated":true}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected power action auth gate, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPowerActionUsesPrivilegedBroker(t *testing.T) {
	server := testServer(t)
	var request privileged.Request
	server.brokerExec = func(_ context.Context, value privileged.Request) error {
		request = value
		return nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/power/action", strings.NewReader(`{"action":"reboot","reauthenticated":true}`)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected power action to be accepted, got %d: %s", response.Code, response.Body.String())
	}
	if request.Operation != "power.shutdown" || request.OperationID == "" || request.RequestedState["action"] != "reboot" {
		t.Fatalf("unexpected privileged power request: %#v", request)
	}
}

func TestShutdownPowerUsesPrivilegedBroker(t *testing.T) {
	server := testServer(t)
	var request privileged.Request
	server.brokerExec = func(_ context.Context, value privileged.Request) error {
		request = value
		return nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/power/shutdown", strings.NewReader(`{"action":"poweroff","reauthenticated":true}`)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected shutdown to be accepted, got %d: %s", response.Code, response.Body.String())
	}
	if request.Operation != "power.shutdown" || request.OperationID == "" || request.RequestedState["action"] != "poweroff" {
		t.Fatalf("unexpected privileged shutdown request: %#v", request)
	}
}
