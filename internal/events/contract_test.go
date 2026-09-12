package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestEncodePreservesObservabilityEnvelope(t *testing.T) {
	want := model.Event{
		SchemaVersion: 1,
		ID:            "evt-contract",
		Type:          "storage.operation.completed",
		Timestamp:     time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC),
		Severity:      "warning",
		CorrelationID: "corr-contract",
		OperationID:   "op-contract",
		PlanHash:      "plan-contract",
		Actor:         "admin",
		Generation:    7,
		Resource:      &model.ResourceRef{Type: "disk", ID: "wwn:test"},
		Data:          map[string]any{"result": "verified"},
	}

	encoded, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	var got model.Event
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != want.SchemaVersion || got.ID != want.ID || got.Type != want.Type || got.CorrelationID != want.CorrelationID || got.OperationID != want.OperationID || got.PlanHash != want.PlanHash || got.Actor != want.Actor || got.Generation != want.Generation {
		t.Fatalf("observability envelope changed during SSE encoding: got %#v want %#v", got, want)
	}
	if got.Resource == nil || *got.Resource != *want.Resource || got.Data["result"] != "verified" {
		t.Fatalf("event payload changed during SSE encoding: got %#v", got)
	}
}
