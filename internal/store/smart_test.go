package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestSMARTSamplesRoundTripAndPrune(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC().Truncate(time.Millisecond)
	temperature := 37.0
	if err := database.SaveSMARTSample(model.SMARTSample{DiskID: "disk-a", CapturedAt: now, TemperatureC: &temperature, Summary: model.SmartSummary{Overall: model.Healthy, PendingSectors: 2}}); err != nil {
		t.Fatal(err)
	}
	values, err := database.SMARTSamples("disk-a", now.Add(-time.Hour), 10)
	if err != nil || len(values) != 1 {
		t.Fatalf("SMART sample round trip failed: %#v %v", values, err)
	}
	if values[0].TemperatureC == nil || *values[0].TemperatureC != temperature || values[0].Summary.PendingSectors != 2 {
		t.Fatalf("SMART sample fields lost: %#v", values[0])
	}
	if err := database.PruneSMARTSamples(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	values, err = database.SMARTSamples("disk-a", now.Add(-time.Hour), 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("SMART sample was not pruned: %#v %v", values, err)
	}
}
