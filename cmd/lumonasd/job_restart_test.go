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
		ID:            "job-restart",
		CorrelationID: "corr-restart",
		Actor:         "admin",
		Type:          "snapraid.sync",
		Title:         "SnapRAID sync",
		State:         "running",
		CreatedAt:     time.Now().UTC(),
		Progress:      &progress,
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
	events, err := server.store.Events(20)
	if err != nil {
		t.Fatal(err)
	}
	var restartEvent *model.Event
	for index := range events {
		if events[index].Type == "job.state_changed" && events[index].OperationID == job.ID {
			restartEvent = &events[index]
			break
		}
	}
	if restartEvent == nil || restartEvent.CorrelationID != job.CorrelationID || restartEvent.Actor != job.Actor || restartEvent.Data["reason"] != "daemon_restart" {
		t.Fatalf("restart failure event lost tracing metadata: %#v", restartEvent)
	}
	server.ensureRestartedJobs()
	events, err = server.store.Events(20)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Type == "job.state_changed" && event.OperationID == job.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("restart reconciliation emitted %d duplicate events", count)
	}
}
