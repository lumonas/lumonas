package main

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestEnsureRestartedJobsFailsClosedForInterruptedWork(t *testing.T) {
	server := testServer(t)
	progress := 42.0
	job := model.Job{
		ID:        "job-restart",
		Type:      "snapraid.sync",
		Title:     "SnapRAID sync",
		State:     "running",
		CreatedAt: time.Now().UTC(),
		Progress:  &progress,
	}
	if err := server.store.SaveJob(job); err != nil {
		t.Fatal(err)
	}

	server.ensureRestartedJobs()

	restarted, err := server.store.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.State != "failed" || restarted.Error != "daemon restarted before the job completed" || restarted.FinishedAt == nil {
		t.Fatalf("interrupted job was not failed closed: %#v", restarted)
	}
}
