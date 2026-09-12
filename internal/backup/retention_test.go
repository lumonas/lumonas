package backup

import (
	"testing"
	"time"
)

func TestRetainedRunsKeepsGenerationDailyAndMonthlyWindows(t *testing.T) {
	base := time.Date(2026, 9, 12, 3, 0, 0, 0, time.UTC)
	runs := []Run{
		{ID: "new", Generation: 4, State: "verified", StartedAt: base},
		{ID: "day", Generation: 3, State: "verified", StartedAt: base.Add(-24 * time.Hour)},
		{ID: "month", Generation: 2, State: "verified", StartedAt: base.AddDate(0, -1, 0)},
		{ID: "failed", Generation: 1, State: "failed", StartedAt: base.AddDate(0, -2, 0)},
	}
	values := RetainedRuns(runs, RetentionPolicy{Generations: 1, Daily: 2, Monthly: 2})
	if len(values) != 3 || values[0].ID != "new" || values[1].ID != "day" || values[2].ID != "month" {
		t.Fatalf("unexpected retention result %#v", values)
	}
}
