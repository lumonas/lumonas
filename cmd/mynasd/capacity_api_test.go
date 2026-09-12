package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCapacityForecastRejectsInvalidWindow(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/capacity/forecast?days=0", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCapacityForecastReturnsEmptyHistoryWithoutPredicting(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/capacity/forecast", nil))
	if response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
}
