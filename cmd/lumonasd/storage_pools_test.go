package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestPoolPlanPersistsAndRejectsReplacedDiskOnConfirmation(t *testing.T) {
	server := testServer(t)
	disk := model.Disk{ID: "wwn:a", CurrentPath: "/dev/sda", WWN: "a", Serial: "serial-a", Model: "TestDisk", SizeBytes: 100, Filesystem: "xfs", Health: model.Healthy, LastSeen: time.Now().UTC()}
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{disk}, nil }
	planResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(planResponse, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/plan", strings.NewReader(`{"name":"media","diskIds":["wwn:a"]}`)))
	if planResponse.Code != http.StatusCreated {
		t.Fatalf("expected pool plan creation, got %d: %s", planResponse.Code, planResponse.Body.String())
	}
	var plan struct {
		OperationID string `json:"operationId"`
		PlanHash    string `json:"planHash"`
	}
	if err := json.NewDecoder(planResponse.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	disk.SizeBytes = 101
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{disk}, nil }
	confirm := httptest.NewRecorder()
	server.routes().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/"+plan.OperationID+"/confirm", strings.NewReader(`{"planHash":"`+plan.PlanHash+`","reauthenticated":true,"storageSafetyUnlocked":true}`)))
	if confirm.Code != http.StatusConflict || !strings.Contains(confirm.Body.String(), "capacity mismatch") {
		t.Fatalf("expected replaced disk rejection, got %d: %s", confirm.Code, confirm.Body.String())
	}
}

func TestPoolPlanCreationIsIdempotent(t *testing.T) {
	server := testServer(t)
	disk := model.Disk{ID: "wwn:a", CurrentPath: "/dev/sda", WWN: "a", SizeBytes: 100, Filesystem: "xfs", Health: model.Healthy, LastSeen: time.Now().UTC()}
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{disk}, nil }
	first := httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/plan", strings.NewReader(`{"name":"media","diskIds":["wwn:a"]}`))
	first.Header.Set("Idempotency-Key", "pool-plan-1")
	firstResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(firstResponse, first)
	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", firstResponse.Code, firstResponse.Body.String())
	}
	var firstPlan struct {
		OperationID string `json:"operationId"`
	}
	if err := json.NewDecoder(firstResponse.Body).Decode(&firstPlan); err != nil {
		t.Fatal(err)
	}
	retry := httptest.NewRequest(http.MethodPost, "/api/v1/storage/pools/plan", strings.NewReader(`{"name":"media","diskIds":["wwn:a"]}`))
	retry.Header.Set("Idempotency-Key", "pool-plan-1")
	retryResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(retryResponse, retry)
	if retryResponse.Code != http.StatusOK || !strings.Contains(retryResponse.Body.String(), firstPlan.OperationID) {
		t.Fatalf("expected original plan on retry, got %d: %s", retryResponse.Code, retryResponse.Body.String())
	}
}
