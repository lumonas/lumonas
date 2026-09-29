package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestRunningSMARTJobCanBeCancelledSafely(t *testing.T) {
	server := testServer(t)
	job := model.Job{ID: "smart-running", Type: "smart.short", ResourceID: "wwn:test", State: "running", CreatedAt: time.Now().UTC()}
	if err := server.store.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.runningJobCancels = map[string]context.CancelFunc{job.ID: cancel}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+job.ID+"/cancel", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("cancel running SMART status %d: %s", response.Code, response.Body.String())
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("running job context was not cancelled")
	}
}

func TestSnapRAIDRunningJobRejectsUnsafeCancellation(t *testing.T) {
	server := testServer(t)
	job := model.Job{ID: "snapraid-running", Type: "snapraid.sync", ResourceID: "protection", State: "running", CreatedAt: time.Now().UTC()}
	if err := server.store.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+job.ID+"/cancel", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("unsafe SnapRAID cancellation status %d: %s", response.Code, response.Body.String())
	}
}
