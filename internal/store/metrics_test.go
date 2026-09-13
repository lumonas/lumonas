package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestSystemMetricSamplesRoundTripNewestFirst(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var migration int
	if err := database.db.QueryRow(`SELECT version FROM schema_migrations WHERE version=9`).Scan(&migration); err != nil || migration != 9 {
		t.Fatalf("system metrics schema migration was not recorded: %d err=%v", migration, err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for index := 0; index < 3; index++ {
		if err := database.SaveSystemMetricSample(model.SystemMetricSample{
			CapturedAt: now.Add(time.Duration(index) * time.Minute),
			Metrics:    model.SystemMetrics{CPUPercent: float64(index), RAMTotalBytes: 1024},
		}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := database.SystemMetricSamples(now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || !items[0].CapturedAt.After(items[1].CapturedAt) || items[0].Metrics.CPUPercent != 2 {
		t.Fatalf("unexpected metric history: %#v", items)
	}
}

func TestSystemMetricSamplesPruneByAge(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC().Truncate(time.Second)
	for _, captured := range []time.Time{now.Add(-2 * time.Hour), now.Add(-time.Hour)} {
		if err := database.SaveSystemMetricSample(model.SystemMetricSample{CapturedAt: captured, Metrics: model.SystemMetrics{RAMTotalBytes: 2048}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.PruneSystemMetricSamples(now.Add(-90 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	items, err := database.SystemMetricSamples(now.Add(-3*time.Hour), 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("unexpected pruned metric history: %#v err=%v", items, err)
	}
}
