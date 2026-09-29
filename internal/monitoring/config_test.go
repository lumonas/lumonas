package monitoring

import (
	"testing"
	"time"
)

func TestNextOccurrenceDaily(t *testing.T) {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	schedule := Schedule{Kind: ScheduleDaily, TimeOfDay: "02:00"}
	// Before the scheduled time: fires the same day.
	now := time.Date(2026, time.March, 10, 1, 30, 0, 0, location)
	next := NextOccurrence(schedule, now)
	expected := time.Date(2026, time.March, 10, 2, 0, 0, 0, location)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}
	// After the scheduled time: fires tomorrow, same wall clock.
	now = time.Date(2026, time.March, 10, 2, 0, 0, 0, location)
	next = NextOccurrence(schedule, now)
	expected = time.Date(2026, time.March, 11, 2, 0, 0, 0, location)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}
}

func TestNextOccurrenceWeekly(t *testing.T) {
	schedule := Schedule{Kind: ScheduleWeekly, Weekday: "sunday", TimeOfDay: "03:00"}
	// 2026-03-10 is a Tuesday; the next Sunday is 2026-03-15.
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	next := NextOccurrence(schedule, now)
	expected := time.Date(2026, time.March, 15, 3, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}
	// Sunday after 03:00 rolls to the following week.
	now = time.Date(2026, time.March, 15, 3, 0, 0, 0, time.UTC)
	next = NextOccurrence(schedule, now)
	expected = time.Date(2026, time.March, 22, 3, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}
}

func TestNextOccurrenceEventIsZero(t *testing.T) {
	if next := NextOccurrence(Schedule{Kind: ScheduleEvent}, time.Now()); !next.IsZero() {
		t.Fatalf("event schedules have no occurrence, got %v", next)
	}
}

func TestMarkStartedAdvancesNextDue(t *testing.T) {
	now := time.Date(2026, time.March, 10, 2, 0, 0, 0, time.UTC)
	schedule := Schedule{Kind: ScheduleDaily, TimeOfDay: "02:00"}
	schedule.MarkStarted(now)
	if schedule.LastStartedAt == nil || !schedule.LastStartedAt.Equal(now) {
		t.Fatalf("expected last started at %v, got %v", now, schedule.LastStartedAt)
	}
	expected := time.Date(2026, time.March, 11, 2, 0, 0, 0, time.UTC)
	if schedule.NextDueAt == nil || !schedule.NextDueAt.Equal(expected) {
		t.Fatalf("expected next due %v, got %v", expected, schedule.NextDueAt)
	}
}

func TestDescribeNext(t *testing.T) {
	now := time.Date(2026, time.March, 10, 1, 0, 0, 0, time.UTC)
	if got := (Schedule{Kind: ScheduleEvent}).DescribeNext(now); got != "on change" {
		t.Fatalf("unexpected event description %q", got)
	}
	if got := (Schedule{Kind: ScheduleDaily, TimeOfDay: "02:00", Enabled: false}).DescribeNext(now); got != "paused" {
		t.Fatalf("unexpected paused description %q", got)
	}
	due := now.Add(9 * time.Hour)
	if got := (Schedule{Kind: ScheduleDaily, TimeOfDay: "02:00", Enabled: true, NextDueAt: &due}).DescribeNext(now); got != "in 9h 0m" {
		t.Fatalf("unexpected duration description %q", got)
	}
}

func TestScheduleValidation(t *testing.T) {
	valid := Schedule{ID: "sched-1", Name: "Sync", JobType: "snapraid.sync", Kind: ScheduleDaily, TimeOfDay: "02:00"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid schedule, got %v", err)
	}
	missingName := valid
	missingName.Name = ""
	if err := missingName.Validate(); err == nil {
		t.Fatal("expected missing name to fail validation")
	}
	badJobType := valid
	badJobType.JobType = "docker.deploy"
	if err := badJobType.Validate(); err == nil {
		t.Fatal("expected unsupported job type to fail validation")
	}
	snapshot := Schedule{ID: "sched-snapshot", Name: "Snapshots", JobType: "snapshot.create", Kind: ScheduleDaily, TimeOfDay: "01:30", SnapshotKind: "btrfs", SnapshotSource: "/srv/pools/media", SnapshotKeep: 7}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("valid snapshot schedule rejected: %v", err)
	}
	lockedSnapshot := snapshot
	lockedSnapshot.SnapshotLockDays = 3650
	if err := lockedSnapshot.Validate(); err != nil {
		t.Fatalf("maximum snapshot lock rejected: %v", err)
	}
	lockedSnapshot.SnapshotLockDays = 3651
	if err := lockedSnapshot.Validate(); err == nil {
		t.Fatal("excessive snapshot lock unexpectedly passed")
	}
	missingSnapshotSource := snapshot
	missingSnapshotSource.SnapshotSource = ""
	if err := missingSnapshotSource.Validate(); err == nil {
		t.Fatal("snapshot schedule without a source unexpectedly passed")
	}
	invalidSnapshotKeep := snapshot
	invalidSnapshotKeep.SnapshotKeep = 0
	if err := invalidSnapshotKeep.Validate(); err == nil {
		t.Fatal("snapshot schedule without bounded retention unexpectedly passed")
	}
	badTime := valid
	badTime.TimeOfDay = "25:00"
	if err := badTime.Validate(); err == nil {
		t.Fatal("expected invalid time of day to fail validation")
	}
	weeklyWithoutWeekday := Schedule{ID: "sched-2", Name: "Scrub", JobType: "snapraid.scrub", Kind: ScheduleWeekly, TimeOfDay: "03:00"}
	if err := weeklyWithoutWeekday.Validate(); err == nil {
		t.Fatal("expected weekly schedule without weekday to fail validation")
	}
	event := Schedule{ID: "sched-3", Name: "Config snapshot", JobType: "config.snapshot", Kind: ScheduleEvent}
	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid event schedule, got %v", err)
	}
}

func TestHumanize(t *testing.T) {
	daily := Schedule{Kind: ScheduleDaily, TimeOfDay: "02:00"}
	daily.Humanize()
	if daily.Schedule != "Daily at 02:00" {
		t.Fatalf("unexpected daily cadence %q", daily.Schedule)
	}
	weekly := Schedule{Kind: ScheduleWeekly, Weekday: "saturday", TimeOfDay: "04:00"}
	weekly.Humanize()
	if weekly.Schedule != "Saturdays at 04:00" {
		t.Fatalf("unexpected weekly cadence %q", weekly.Schedule)
	}
	event := Schedule{Kind: ScheduleEvent}
	event.Humanize()
	if event.Schedule != "After every change" {
		t.Fatalf("unexpected event cadence %q", event.Schedule)
	}
}
