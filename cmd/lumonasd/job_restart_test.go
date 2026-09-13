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
		OperationID:   "op-restart",
		PlanHash:      "plan-restart",
		Actor:         "admin",
		Generation:    7,
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
		if events[index].Type == "job.state_changed" && events[index].OperationID == job.OperationID {
			restartEvent = &events[index]
			break
		}
	}
	if restartEvent == nil || restartEvent.CorrelationID != job.CorrelationID || restartEvent.OperationID != job.OperationID || restartEvent.PlanHash != job.PlanHash || restartEvent.Generation != job.Generation || restartEvent.Actor != job.Actor || restartEvent.Data["reason"] != "daemon_restart" {
		t.Fatalf("restart failure event lost tracing metadata: %#v", restartEvent)
	}
	server.ensureRestartedJobs()
	events, err = server.store.Events(20)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Type == "job.state_changed" && event.OperationID == job.OperationID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("restart reconciliation emitted %d duplicate events", count)
	}
}

func TestPublishActorSetsEventEnvelopeWithoutPayloadMutation(t *testing.T) {
	server := testServer(t)
	server.publishActor("admin", "settings.changed", "info", nil, map[string]any{"section": "runtime"})
	events, err := server.store.Events(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Actor != "admin" {
		t.Fatalf("event actor was not promoted: %#v", events)
	}
	if _, ok := events[0].Data["actor"]; ok {
		t.Fatalf("actor leaked into event payload: %#v", events[0].Data)
	}
}
