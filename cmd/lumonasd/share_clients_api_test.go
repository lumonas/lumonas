package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisconnectShareClientRequiresExplicitConfirmation(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/shares/clients/disconnect", strings.NewReader(`{"address":"192.0.2.41"}`)))
	if response.Code != http.StatusLocked || !strings.Contains(response.Body.String(), "explicit confirmation") {
		t.Fatalf("disconnect request without confirmation was not blocked: %d %s", response.Code, response.Body.String())
	}
}

func TestDisconnectShareClientRejectsInvalidAddress(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/shares/clients/disconnect", strings.NewReader(`{"address":"192.0.2.41; reboot","confirmed":true}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid client address was accepted: %d %s", response.Code, response.Body.String())
	}
}
