package events

import (
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestMetadataFromDataPromotesNestedJobTracing(t *testing.T) {
	job := model.Job{ID: "job-1", CorrelationID: "corr-1"}
	got := MetadataFromData(map[string]any{"job": job})
	if got.CorrelationID != "corr-1" || got.OperationID != "job-1" {
		t.Fatalf("nested job metadata was not promoted: %#v", got)
	}
}

func TestMetadataFromDataPrefersExplicitEnvelopeFields(t *testing.T) {
	job := model.Job{ID: "job-1", CorrelationID: "corr-job"}
	got := MetadataFromData(map[string]any{
		"job":           job,
		"correlationId": "corr-explicit",
		"operationId":   "op-explicit",
		"planHash":      "plan-explicit",
	})
	want := Metadata{CorrelationID: "corr-explicit", OperationID: "op-explicit", PlanHash: "plan-explicit"}
	if got != want {
		t.Fatalf("explicit event metadata was overwritten: got %#v want %#v", got, want)
	}
}
