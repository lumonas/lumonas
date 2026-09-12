package services

import (
	"context"
	"testing"
)

func TestCollectReturnsEveryRequestedService(t *testing.T) {
	values := Collect(context.Background(), []string{"service-that-does-not-exist.service"})
	if len(values) != 1 || values[0].Name == "" || values[0].Active {
		t.Fatalf("unexpected service status %#v", values)
	}
	if values[0].ID == "" {
		t.Fatal("expected status to carry the unit name as id")
	}
	if values[0].State != "running" && values[0].State != "stopped" && values[0].State != "degraded" {
		t.Fatalf("state %q is not normalized to the UI vocabulary", values[0].State)
	}
}

func TestNormalizeState(t *testing.T) {
	cases := map[string]string{
		"active":       "running",
		"reloading":    "running",
		"inactive":     "stopped",
		"deactivating": "stopped",
		"activating":   "degraded",
		"failed":       "degraded",
		"unknown":      "degraded",
		"":             "degraded",
		"unexpected":   "degraded",
	}
	for raw, want := range cases {
		if got := normalizeState(raw); got != want {
			t.Errorf("normalizeState(%q) = %q, want %q", raw, got, want)
		}
	}
}
