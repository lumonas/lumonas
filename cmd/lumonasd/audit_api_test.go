package main

import (
	"net/http/httptest"
	"testing"
)

func TestParseAuditFiltersAndRejectInvalidRange(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/audit?actor=admin&action=share.update&outcome=committed&from=2026-09-01T00:00:00Z&to=2026-09-28T23:59:59Z", nil)
	filters, err := parseAuditFilters(request)
	if err != nil || filters.Actor != "admin" || filters.Action != "share.update" || filters.Outcome != "committed" || filters.From.IsZero() || filters.To.IsZero() {
		t.Fatalf("parsed filters = %#v err=%v", filters, err)
	}
	invalid := httptest.NewRequest("GET", "/api/v1/audit?from=tomorrow", nil)
	if _, err := parseAuditFilters(invalid); err == nil {
		t.Fatal("invalid date filter was accepted")
	}
	badRange := httptest.NewRequest("GET", "/api/v1/audit?from=2026-09-28T00:00:00Z&to=2026-09-01T00:00:00Z", nil)
	if _, err := parseAuditFilters(badRange); err == nil {
		t.Fatal("inverted date range was accepted")
	}
}

func TestAuditCSVFormulaProtection(t *testing.T) {
	for _, value := range []string{"=1+1", "  @SUM(A1)", "+cmd", "-cmd"} {
		if safeCSV(value) == value {
			t.Fatalf("CSV formula prefix was not escaped: %q", value)
		}
	}
	if got := safeCSV("ordinary text"); got != "ordinary text" {
		t.Fatalf("ordinary CSV cell changed: %q", got)
	}
}
