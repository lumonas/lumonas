package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/events"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
)

func testServer(t *testing.T) *apiServer {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.SetMeta("nas_uuid", "nas-test"); err != nil {
		t.Fatal(err)
	}
	return &apiServer{store: db, hub: events.NewHub(), version: "test", acknowledged: make(map[string]bool), diskFunc: func() ([]model.Disk, error) {
		return []model.Disk{{ID: "wwn:test", Name: "sda", Role: "unknown", Health: model.Healthy, LastSeen: time.Now().UTC()}}, nil
	}}
}

func TestAPIHealthAndDiskIdentity(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	var disks []model.Disk
	if err := json.NewDecoder(response.Body).Decode(&disks); err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 || disks[0].ID != "wwn:test" {
		t.Fatalf("unexpected disks %#v", disks)
	}
}

func TestUnsupportedJobFailsClosed(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", io.NopCloser(strings.NewReader(`{"type":"snapraid.sync","resourceId":"wwn:test"}`)))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", response.Code)
	}
}

func TestStoragePlanRequiresStableIdentityAndSafetyUnlock(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storage/operations/plan", strings.NewReader(`{"action":"filesystem.format","diskId":"wwn:test"}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected plan creation, got %d: %s", response.Code, response.Body.String())
	}
	var plan struct {
		OperationID string `json:"operationId"`
		PlanHash    string `json:"planHash"`
	}
	if err := json.NewDecoder(response.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	confirm := httptest.NewRequest(http.MethodPost, "/api/v1/storage/operations/"+plan.OperationID+"/confirm", strings.NewReader(`{"planHash":"`+plan.PlanHash+`"}`))
	confirmed := httptest.NewRecorder()
	server.routes().ServeHTTP(confirmed, confirm)
	if confirmed.Code != http.StatusLocked {
		t.Fatalf("expected safety lock, got %d", confirmed.Code)
	}
}

func TestAuthRequiredProtectsAPIButNotHealth(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	apiResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/v1/server", nil))
	if apiResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected API auth, got %d", apiResponse.Code)
	}
	healthResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(healthResponse, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("expected health endpoint, got %d", healthResponse.Code)
	}
}
