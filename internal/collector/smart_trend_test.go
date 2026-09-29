package collector

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestSMARTTrendReportsIncreasingErrorCounters(t *testing.T) {
	now := time.Now().UTC()
	samples := []model.SMARTSample{
		{DiskID: "disk-a", CapturedAt: now.Add(-48 * time.Hour), Summary: model.SmartSummary{Overall: model.Healthy, ReallocatedSectors: 1, PendingSectors: 0}},
		{DiskID: "disk-a", CapturedAt: now, Summary: model.SmartSummary{Overall: model.Healthy, ReallocatedSectors: 5, PendingSectors: 2}},
	}
	trend := SMARTTrend(samples)
	if trend.Status != model.Warning || trend.ReallocatedSlope <= 0 || trend.PendingSlope <= 0 {
		t.Fatalf("unexpected SMART trend: %#v", trend)
	}
	if trend.SampleCount != 2 || trend.Since == nil {
		t.Fatalf("trend history metadata missing: %#v", trend)
	}
}

func TestSMARTTrendPreservesCriticalStatus(t *testing.T) {
	trend := SMARTTrend([]model.SMARTSample{{DiskID: "disk-a", CapturedAt: time.Now().UTC(), Summary: model.SmartSummary{Overall: model.Critical}}})
	if trend.Status != model.Critical || trend.Summary != "First SMART sample recorded" {
		t.Fatalf("unexpected critical trend: %#v", trend)
	}
}
