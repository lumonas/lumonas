package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitFileIntegrityJob(t *testing.T, server *apiServer, id string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err := server.store.Job(id)
		if err == nil && (job.State == "successful" || job.State == "failed") {
			if job.State != "successful" {
				t.Fatalf("integrity job failed: %#v", job)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("integrity scan job did not finish")
}

func TestFileIntegrityBaselineAndVerificationReport(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, _ := server.store.ManagedShare(shareID)
	dir := filepath.Join(share.Path, "integrity-fixture")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"change.txt": "original", "missing.txt": "remove me", "same.txt": "stable"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/files/integrity/baseline", strings.NewReader(`{"shareId":"`+shareID+`"}`)))
	if create.Code != http.StatusAccepted {
		t.Fatalf("baseline status %d: %s", create.Code, create.Body.String())
	}
	var baselineJob struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(create.Body).Decode(&baselineJob); err != nil {
		t.Fatal(err)
	}
	waitFileIntegrityJob(t, server, baselineJob.ID)
	if err := os.WriteFile(filepath.Join(dir, "change.txt"), []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "missing.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	verify := httptest.NewRecorder()
	server.routes().ServeHTTP(verify, httptest.NewRequest(http.MethodPost, "/api/v1/files/integrity/verify", strings.NewReader(`{"shareId":"`+shareID+`"}`)))
	if verify.Code != http.StatusAccepted {
		t.Fatalf("verify status %d: %s", verify.Code, verify.Body.String())
	}
	var verifyJob struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(verify.Body).Decode(&verifyJob); err != nil {
		t.Fatal(err)
	}
	waitFileIntegrityJob(t, server, verifyJob.ID)
	status := httptest.NewRecorder()
	server.routes().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/files/integrity?shareId="+shareID, nil))
	if status.Code != http.StatusOK {
		t.Fatalf("status response %d: %s", status.Code, status.Body.String())
	}
	if !strings.Contains(status.Body.String(), `"changedCount":1`) || !strings.Contains(status.Body.String(), `"missingCount":1`) || !strings.Contains(status.Body.String(), `"addedCount":1`) {
		t.Fatalf("integrity differences not reported: %s", status.Body.String())
	}
}

func TestIntegrityVerifyRequiresBaseline(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/files/integrity/verify", strings.NewReader(`{"shareId":"`+shareID+`"}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("verify without baseline status %d: %s", response.Code, response.Body.String())
	}
}
