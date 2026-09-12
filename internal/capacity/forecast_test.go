package capacity

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestBuildRequiresHistoryAndForecastsGrowth(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snapshots := []model.CapacitySnapshot{
		{ResourceID: "/srv/pools/media", CapturedAt: start, TotalBytes: 1000, UsedBytes: 100},
		{ResourceID: "/srv/pools/media", CapturedAt: start.Add(24 * time.Hour), TotalBytes: 1000, UsedBytes: 200},
		{ResourceID: "/srv/pools/media", CapturedAt: start.Add(48 * time.Hour), TotalBytes: 1000, UsedBytes: 300},
	}
	forecast := Build("/srv/pools/media", snapshots, start.Add(3*24*time.Hour))
	if !forecast.Available || forecast.GrowthBytesPerDay != 100 {
		t.Fatalf("unexpected forecast: %#v", forecast)
	}
	if forecast.DaysToNinetyPercent == nil || *forecast.DaysToNinetyPercent != 6 {
		t.Fatalf("unexpected days to threshold: %#v", forecast.DaysToNinetyPercent)
	}
}

func TestBuildDoesNotPredictWithShortHistory(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	forecast := Build("pool", []model.CapacitySnapshot{
		{CapturedAt: start, TotalBytes: 100, UsedBytes: 10},
		{CapturedAt: start.Add(12 * time.Hour), TotalBytes: 100, UsedBytes: 20},
		{CapturedAt: start.Add(20 * time.Hour), TotalBytes: 100, UsedBytes: 30},
	}, start.Add(24*time.Hour))
	if forecast.Available || forecast.DaysToNinetyPercent != nil {
		t.Fatalf("short history should not predict: %#v", forecast)
	}
}
