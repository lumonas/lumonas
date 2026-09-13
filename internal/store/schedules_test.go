package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/monitoring"
)

func TestJobSchedulesSeedDefaults(t *testing.T) {
	db, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schedules, err := db.JobSchedules(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 6 {
		t.Fatalf("expected 6 seeded schedules, got %d", len(schedules))
	}
	var sync monitoring.Schedule
	for _, schedule := range schedules {
		if schedule.ID == "sched-sync" {
			sync = schedule
		}
		if schedule.Schedule == "" {
			t.Fatalf("schedule %s has empty humanized cadence", schedule.ID)
		}
		if schedule.Next == "" {
			t.Fatalf("schedule %s has empty next description", schedule.ID)
		}
	}
	if sync.JobType != "snapraid.sync" || sync.Kind != monitoring.ScheduleDaily || sync.TimeOfDay != "02:00" || !sync.Enabled {
		t.Fatalf("unexpected sync schedule: %#v", sync)
	}
	if sync.Schedule != "Daily at 02:00" {
		t.Fatalf("unexpected cadence %q", sync.Schedule)
	}
}

func TestSaveJobScheduleRoundTrip(t *testing.T) {
	db, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	schedules, err := db.JobSchedules(now)
	if err != nil {
		t.Fatal(err)
	}
	var scrub monitoring.Schedule
	for _, schedule := range schedules {
		if schedule.ID == "sched-scrub" {
			scrub = schedule
		}
	}
	scrub.Enabled = false
	due := now.Add(48 * time.Hour)
	scrub.NextDueAt = &due
	if err := db.SaveJobSchedule(scrub); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.JobSchedule("sched-scrub", now)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Enabled {
		t.Fatal("expected scrub schedule to remain disabled")
	}
	if loaded.NextDueAt == nil || !loaded.NextDueAt.Equal(due) {
		t.Fatalf("expected next due %v, got %v", due, loaded.NextDueAt)
	}
	if loaded.DescribeNext(now) != "paused" {
		t.Fatalf("unexpected next description %q", loaded.DescribeNext(now))
	}
}

func TestSaveJobScheduleRejectsInvalid(t *testing.T) {
	db, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	invalid := monitoring.Schedule{ID: "sched-bad", Name: "Bad", JobType: "docker.deploy", Kind: monitoring.ScheduleDaily, TimeOfDay: "02:00"}
	if err := db.SaveJobSchedule(invalid); err == nil {
		t.Fatal("expected invalid schedule to be rejected")
	}
}
