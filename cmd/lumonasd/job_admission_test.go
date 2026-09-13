package main

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestJobResourceKeyGroupsMutuallyExclusiveWork(t *testing.T) {
	tests := []struct {
		left, right, want string
	}{
		{"snapraid.sync", "protection", "protection"},
		{"snapraid.scrub", "protection", "protection"},
		{"smart.short", "wwn:a", "disk:wwn:a"},
		{"smart.extended", "wwn:a", "disk:wwn:a"},
	}
	for _, test := range tests {
		if got := jobResourceKey(test.left, test.right); got != test.want {
			t.Fatalf("jobResourceKey(%q, %q) = %q, want %q", test.left, test.right, got, test.want)
		}
	}
	if jobResourceKey("smart.short", "wwn:a") == jobResourceKey("smart.short", "wwn:b") {
		t.Fatal("different stable disks must not share a SMART resource lock")
	}
}

func TestAdmitJobRejectsConflictingActiveResource(t *testing.T) {
	server := testServer(t)
	active := model.Job{ID: "job-active", Type: "snapraid.sync", ResourceID: "protection", State: "running", CreatedAt: time.Now().UTC()}
	if err := server.store.SaveJob(active); err != nil {
		t.Fatal(err)
	}
	job := model.Job{ID: "job-new", Type: "snapraid.scrub", ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
	err := server.admitJob(&job)
	if !isJobResourceBusy(err) || !strings.Contains(err.Error(), active.ID) {
		t.Fatalf("expected protection conflict, got %v", err)
	}
	jobs, err := server.store.Jobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("conflicting job was persisted: %#v", jobs)
	}
}

func TestAdmitJobAllowsTerminalAndDifferentDiskJobs(t *testing.T) {
	server := testServer(t)
	for _, job := range []model.Job{
		{ID: "job-done", Type: "smart.short", ResourceID: "wwn:a", State: "successful", CreatedAt: time.Now().UTC()},
		{ID: "job-other", Type: "smart.short", ResourceID: "wwn:b", State: "running", CreatedAt: time.Now().UTC()},
	} {
		if err := server.store.SaveJob(job); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.admitJob(&model.Job{ID: "job-new", Type: "smart.extended", ResourceID: "wwn:a", State: "queued", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("terminal job should release its resource: %v", err)
	}
	if err := server.admitJob(&model.Job{ID: "job-other-disk", Type: "smart.short", ResourceID: "wwn:c", State: "queued", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("different disk should be admitted: %v", err)
	}
}
