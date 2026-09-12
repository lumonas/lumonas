package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNetworkConfigurationAPIRequiresReauthenticationAndPersistsTypedState(t *testing.T) {
	server := testServer(t)

	locked := httptest.NewRecorder()
	server.routes().ServeHTTP(locked, httptest.NewRequest(http.MethodPatch, "/api/v1/network/connections/lan", strings.NewReader(`{"uuid":"12345678-1234","name":"LAN","interface":"en0","enabled":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"}}`)))
	if locked.Code != http.StatusLocked {
		t.Fatalf("expected reauthentication lock, got %d: %s", locked.Code, locked.Body.String())
	}

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/network/connections/lan", strings.NewReader(`{"uuid":"12345678-1234","name":"LAN","interface":"en0","enabled":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"},"reauthenticated":true}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"lan"`) {
		t.Fatalf("unexpected update response %d: %s", response.Code, response.Body.String())
	}

	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/network/connections", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"interface":"en0"`) {
		t.Fatalf("unexpected list response %d: %s", list.Code, list.Body.String())
	}
}

func TestNetworkConnectionCreateAndDiagnosticJobLifecycle(t *testing.T) {
	server := testServer(t)
	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/network/connections", strings.NewReader(`{"name":"LAN","interface":"en0","enabled":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"},"reauthenticated":true}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", create.Code, create.Body.String())
	}

	diagnostic := httptest.NewRecorder()
	server.routes().ServeHTTP(diagnostic, httptest.NewRequest(http.MethodPost, "/api/v1/network/diagnostics", strings.NewReader(`{"kind":"interfaces"}`)))
	if diagnostic.Code != http.StatusAccepted {
		t.Fatalf("diagnostic status %d: %s", diagnostic.Code, diagnostic.Body.String())
	}
	var queued struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(diagnostic.Body).Decode(&queued); err != nil || queued.ID == "" {
		t.Fatalf("invalid diagnostic job: %#v err=%v", queued, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		result := httptest.NewRecorder()
		server.routes().ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/api/v1/network/diagnostics/"+queued.ID, nil))
		if result.Code != http.StatusOK {
			t.Fatalf("diagnostic get status %d: %s", result.Code, result.Body.String())
		}
		if strings.Contains(result.Body.String(), `"state":"successful"`) || strings.Contains(result.Body.String(), `"state":"failed"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("diagnostic job did not finish: %s", result.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNetworkDiagnosticRejectsUnsupportedKind(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/diagnostics", strings.NewReader(`{"kind":"shell","target":"127.0.0.1"}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected diagnostic validation error, got %d", response.Code)
	}
}
