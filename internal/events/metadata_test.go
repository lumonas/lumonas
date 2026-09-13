package events

import (
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestMetadataFromDataPromotesNestedJobTracing(t *testing.T) {
	job := model.Job{ID: "job-1", CorrelationID: "corr-1", OperationID: "op-1", PlanHash: "plan-1", Actor: "admin", Generation: 7}
	got := MetadataFromData(map[string]any{"job": job})
	if got.CorrelationID != "corr-1" || got.OperationID != "op-1" || got.PlanHash != "plan-1" || got.Generation != 7 {
		t.Fatalf("nested job metadata was not promoted: %#v", got)
	}
	if got.Actor != "admin" {
		t.Fatalf("nested job actor was not promoted: %#v", got)
	}
}

func TestMetadataFromDataPrefersExplicitEnvelopeFields(t *testing.T) {
	job := model.Job{ID: "job-1", CorrelationID: "corr-job", Actor: "nested-actor"}
	got := MetadataFromData(map[string]any{
		"job":           job,
		"correlationId": "corr-explicit",
		"operationId":   "op-explicit",
		"planHash":      "plan-explicit",
		"actor":         "explicit-actor",
	})
	want := Metadata{CorrelationID: "corr-explicit", OperationID: "op-explicit", PlanHash: "plan-explicit", Actor: "explicit-actor", Generation: 0}
	if got != want {
		t.Fatalf("explicit event metadata was overwritten: got %#v want %#v", got, want)
	}
}
