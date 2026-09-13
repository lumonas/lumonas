package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/privileged"
)

func TestRuntimeSettingsExposeLiveState(t *testing.T) {
	server := testServer(t)
	server.runtimeStateFunc = func() map[string]any {
		return map[string]any{
			"zram":  map[string]any{"enabled": true, "sizeBytes": 1073741824, "compressedBytes": 268435456, "ratio": 4.0, "pressure": "medium"},
			"tmpfs": map[string]any{"enabled": true, "sizeBytes": 536870912, "mountPath": "/var/tmp/lumonas-transcode"},
		}
	}

	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("settings fetch failed: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"pressure":"medium"`) {
		t.Fatal("live zram state missing from settings payload")
	}
	if !strings.Contains(rec.Body.String(), `"mountPath":"/var/tmp/lumonas-transcode"`) {
		t.Fatal("tmpfs state missing from settings payload")
	}
}

func TestRuntimePatchRejectsInvalidSizesBeforeProvisioning(t *testing.T) {
	server := testServer(t)
	// The broker is never available in tests: if provisioning were attempted
	// the PATCH would fail with 503 rather than 422.
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"section":"runtime","patch":{"zram":{"enabled":true,"sizeBytes":1048576}}}`)))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "64MiB") {
		t.Fatalf("expected 422 size floor, got %d: %s", response.Code, response.Body.String())
	}

	// Enabling without a size is rejected.
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"section":"runtime","patch":{"tmpfs":{"enabled":true}}}`)))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "sizeBytes is required") {
		t.Fatalf("expected 422 missing size, got %d: %s", response.Code, response.Body.String())
	}
}

func TestRuntimePatchUnknownKeyStillRejected(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"section":"runtime","patch":{"zramTurbo":true}}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown runtime key must stay rejected, got %d", response.Code)
	}
}

func TestRuntimePatchUsesTypedBrokerAndPreservesOtherComponent(t *testing.T) {
	server := testServer(t)
	server.runtimeStateFunc = func() map[string]any { return nil }
	requests := make([]privileged.Request, 0)
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		requests = append(requests, request)
		return nil
	}

	record := httptest.NewRecorder()
	server.routes().ServeHTTP(record, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"section":"runtime","patch":{"zram":{"enabled":true,"sizeBytes":1073741824}}}`)))
	if record.Code != http.StatusOK {
		t.Fatalf("runtime patch failed: %d %s", record.Code, record.Body.String())
	}
	if len(requests) != 2 || requests[0].Operation != "runtime.zram.apply" || requests[1].Operation != "runtime.config.apply" {
		t.Fatalf("unexpected broker requests: %#v", requests)
	}
	if enabled, ok := requests[1].RequestedState["tmpfsEnabled"].(bool); !ok || enabled {
		t.Fatalf("partial zram patch unexpectedly enabled tmpfs: %#v", requests[1].RequestedState)
	}
	if requests[1].RequestedState["zramSizeBytes"] != "1073741824" {
		t.Fatalf("zram desired size was not persisted: %#v", requests[1].RequestedState)
	}
}
