package power

import (
	"testing"
	"time"
)

func TestScheduleValidationAndDueDays(t *testing.T) {
	schedule := Schedule{Enabled: true, Action: "shutdown", Time: "03:15", Days: "weekdays"}
	if err := schedule.Validate(); err != nil {
		t.Fatal(err)
	}
	if !schedule.Due(time.Date(2026, time.January, 5, 3, 15, 0, 0, time.Local)) {
		t.Fatal("Monday schedule should be due")
	}
	if schedule.Due(time.Date(2026, time.January, 4, 3, 15, 0, 0, time.Local)) {
		t.Fatal("Sunday should not match weekday schedule")
	}
}

func TestScheduleRejectsUnsafeValues(t *testing.T) {
	for _, schedule := range []Schedule{
		{Enabled: true, Action: "poweroff", Time: "03:15", Days: "daily"},
		{Enabled: true, Action: "shutdown", Time: "3:15", Days: "daily"},
		{Enabled: true, Action: "shutdown", Time: "03:15", Days: "someday"},
	} {
		if err := schedule.Validate(); err == nil {
			t.Fatalf("unsafe schedule was accepted: %#v", schedule)
		}
	}
}
