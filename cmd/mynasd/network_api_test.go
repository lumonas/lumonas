package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestNetworkDiagnosticRejectsUnsupportedKind(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/diagnostics", strings.NewReader(`{"kind":"shell","target":"127.0.0.1"}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected diagnostic validation error, got %d", response.Code)
	}
}
