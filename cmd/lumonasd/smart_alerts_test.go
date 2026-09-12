package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func smartTestDisk(id string, summary model.SmartSummary) model.Disk {
	return model.Disk{ID: id, Name: "sda", Model: "TestDisk", Serial: id, Role: "data", Health: model.Healthy, LastSeen: time.Now().UTC(), SMART: summary}
}

func TestSMARTAlertsFireAndResolve(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{smartTestDisk("wwn:degraded", model.SmartSummary{
			Overall: model.Warning, ReallocatedSectors: 8, PendingSectors: 12,
		})}, nil
	}

	server.evaluateSMARTAlerts()
	alerts, err := server.store.GeneratedAlerts()
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected one SMART alert, got %d: %#v", len(alerts), alerts)
	}
	if alerts[0].Severity != "warning" || !strings.Contains(alerts[0].Description, "12 pending") {
		t.Fatalf("unexpected alert: %#v", alerts[0])
	}

	// Re-evaluating must not duplicate the alert.
	server.evaluateSMARTAlerts()
	alerts, _ = server.store.GeneratedAlerts()
	if len(alerts) != 1 {
		t.Fatalf("duplicate SMART alert opened: %d", len(alerts))
	}

	// A failed self-assessment escalates to a critical alert.
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{smartTestDisk("wwn:failed", model.SmartSummary{Overall: model.Critical})}, nil
	}
	server.evaluateSMARTAlerts()
	alerts, _ = server.store.GeneratedAlerts()
	found := false
	for _, alert := range alerts {
		if strings.Contains(alert.Title, "self-assessment failed") {
			if alert.Severity != "critical" {
				t.Fatalf("failed SMART should be critical, got %q", alert.Severity)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("critical SMART alert missing: %#v", alerts)
	}

	// Healthy counters resolve the alert.
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{smartTestDisk("wwn:degraded", model.SmartSummary{Overall: model.Healthy})}, nil
	}
	server.evaluateSMARTAlerts()
	alerts, _ = server.store.GeneratedAlerts()
	for _, alert := range alerts {
		if alert.Resource != nil && alert.Resource.ID == "wwn:degraded" && alert.State != "resolved" {
			t.Fatalf("healthy disk did not resolve its SMART alert: %#v", alert)
		}
	}
}

func TestRetireDiskRemovesMissingAlertSource(t *testing.T) {
	t.Setenv("LUMONAS_SNAPRAID_CONFIG", t.TempDir()+"/absent-snapraid.conf")
	server := testServer(t)
	server.authRequired = true
	principal := createManagementUser(t, server, "retire-owner", "correct-horse-battery")
	cookie, token := loginAs(t, server, "retire-owner", "correct-horse-battery")

	if err := server.store.SaveDiskInventory([]model.Disk{{
		ID: "wwn:gone", Name: "sdb", Model: "OldDisk", Serial: "gone-serial", Role: "data",
		LastSeen: time.Now().UTC().Add(-time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{}, nil }

	// The missing disk currently drives a critical alert and health.
	if health := server.serverHealth(); health != model.Critical {
		t.Fatalf("expected critical health while disk missing, got %q", health)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/storage/disks/wwn:gone/retire", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", token)
	req.Header.Set("X-Identity-Actor", principal.ID)
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("retire failed: %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ID      string `json:"id"`
		Retired bool   `json:"retired"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Retired || payload.ID != "wwn:gone" {
		t.Fatalf("unexpected retire response: %#v", payload)
	}

	known, err := server.store.KnownDisks()
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 0 {
		t.Fatalf("inventory still contains the retired disk: %#v", known)
	}
	if health := server.serverHealth(); health == model.Critical {
		t.Fatalf("health stayed critical after retire, got %q", health)
	}

	// Retiring an unknown disk is a 404.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/storage/disks/wwn:never-there/retire", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", token)
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown disk, got %d", rec.Code)
	}
}
