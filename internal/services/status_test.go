package services

import (
	"context"
	"testing"
)

func TestDefaultNamesExposeCompleteApplianceTopology(t *testing.T) {
	want := []string{
		"lumonas-runtime.service",
		"lumonas-privd.service",
		"lumonas-privd-storage.service",
		"lumonas-privd-network.service",
		"lumonas-privd-power.service",
		"lumonas-privd-general.service",
		"lumonas-privd-acme.service",
		"lumonasd.service",
		"lumonas-web.service",
		"lumonas-jobs.target",
		"lumonas-services.target",
		"lumonas-storage.target",
	}
	seen := make(map[string]bool, len(DefaultNames))
	for _, name := range DefaultNames {
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("DefaultNames is missing %q", name)
		}
	}
	if len(seen) != len(DefaultNames) {
		t.Fatal("DefaultNames contains duplicate service identifiers")
	}
}

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
