package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
)

func diskWithTemperature(id string, temperature float64) model.Disk {
	return model.Disk{ID: id, Name: "sda", Model: "Hot Disk", Health: model.Healthy, Temperature: &temperature, LastSeen: time.Now().UTC()}
}

func TestAlertEngineFiresAndResolvesTemperature(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveAlertRule(monitoring.AlertRule{ID: "rule-temp", Name: "Disk temperature high", Condition: "temperature > 45°C for 5 minutes", Severity: "warning", Routes: []string{"web"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{diskWithTemperature("wwn:hot", 52)}, nil }

	server.evaluateDiskTemperatures()

	alerts, err := server.store.GeneratedAlerts()
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || !strings.Contains(alerts[0].Title, "Disk temperature high") {
		t.Fatalf("expected temperature alert, got %#v", alerts)
	}
	rule, err := server.store.AlertRule("rule-temp")
	if err != nil {
		t.Fatal(err)
	}
	if rule.LastTriggeredAt == nil {
		t.Fatal("expected LastTriggeredAt to be stamped")
	}

	// Re-evaluating must not duplicate the open alert.
	server.evaluateDiskTemperatures()
	alerts, _ = server.store.GeneratedAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected deduplicated alert, got %d", len(alerts))
	}

	// Cooling down resolves the alert.
	server.diskFunc = func() ([]model.Disk, error) { return []model.Disk{diskWithTemperature("wwn:hot", 38.5)}, nil }
	server.evaluateDiskTemperatures()
	alerts, _ = server.store.GeneratedAlerts()
	if len(alerts) != 0 {
		t.Fatalf("expected alert resolved, got %#v", alerts)
	}
}

func TestAlertEngineBackupFailureLifecycle(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveAlertRule(monitoring.AlertRule{ID: "rule-backup", Name: "Backup job failed", Condition: "backup job state = failed", Severity: "warning", Routes: []string{"web"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	server.evaluateEventAlert("recovery.backup.failed", map[string]any{"error": "destination unreachable"})
	alerts, _ := server.store.GeneratedAlerts()
	if len(alerts) != 1 || alerts[0].Severity != "warning" {
		t.Fatalf("expected backup failure alert, got %#v", alerts)
	}

	server.evaluateEventAlert("recovery.backup.completed", map[string]any{})
	alerts, _ = server.store.GeneratedAlerts()
	if len(alerts) != 0 {
		t.Fatalf("expected backup alert resolved, got %#v", alerts)
	}
}

func TestAlertEngineSyncStaleness(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveAlertRule(monitoring.AlertRule{ID: "rule-sync", Name: "SnapRAID sync stale", Condition: "no successful sync in 48h", Severity: "attention", Routes: []string{"web"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// No sync recorded yet: nothing fires during onboarding.
	server.evaluateSyncStaleness()
	if alerts, _ := server.store.GeneratedAlerts(); len(alerts) != 0 {
		t.Fatalf("expected no alert without sync history, got %#v", alerts)
	}

	stale := time.Now().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	if err := server.store.SetMeta("snapraid_last_sync_at", stale); err != nil {
		t.Fatal(err)
	}
	server.evaluateSyncStaleness()
	alerts, _ := server.store.GeneratedAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected stale-sync alert, got %#v", alerts)
	}

	fresh := time.Now().Format(time.RFC3339Nano)
	if err := server.store.SetMeta("snapraid_last_sync_at", fresh); err != nil {
		t.Fatal(err)
	}
	server.evaluateSyncStaleness()
	if alerts, _ := server.store.GeneratedAlerts(); len(alerts) != 0 {
		t.Fatalf("expected stale-sync alert resolved, got %#v", alerts)
	}
}

func TestAlertEngineFilesystemCapacityLifecycle(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveAlertRule(monitoring.AlertRule{ID: "rule-filesystem", Name: "Filesystem nearly full", Condition: "filesystem usage above 80%", Severity: "warning", Routes: []string{"web"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server.evaluateFilesystemUsage([]model.FilesystemUsage{{Path: "/var/lib/lumonas", UsedPercent: 96, AvailableBytes: 4, State: "critical"}})
	alerts, _ := server.store.GeneratedAlerts()
	if len(alerts) != 1 || alerts[0].Severity != "critical" || alerts[0].Resource == nil || alerts[0].Resource.Type != "filesystem" {
		t.Fatalf("expected critical filesystem alert, got %#v", alerts)
	}
	server.evaluateFilesystemUsage([]model.FilesystemUsage{{Path: "/var/lib/lumonas", UsedPercent: 50, AvailableBytes: 50, State: "healthy"}})
	alerts, _ = server.store.GeneratedAlerts()
	if len(alerts) != 0 {
		t.Fatalf("expected filesystem alert to resolve, got %#v", alerts)
	}
}

func TestDisabledRuleDoesNotFire(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveAlertRule(monitoring.AlertRule{ID: "rule-backup", Name: "Backup job failed", Condition: "backup job state = failed", Severity: "warning", Routes: []string{"web"}, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	server.evaluateEventAlert("recovery.backup.failed", map[string]any{"error": "x"})
	if alerts, _ := server.store.GeneratedAlerts(); len(alerts) != 0 {
		t.Fatalf("expected disabled rule to stay silent, got %#v", alerts)
	}
}

func TestAlertAckPersistsAcrossViews(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveAlertRule(monitoring.AlertRule{ID: "rule-backup", Name: "Backup job failed", Condition: "backup job state = failed", Severity: "warning", Routes: []string{"web"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server.evaluateEventAlert("recovery.backup.failed", map[string]any{"error": "boom"})
	alerts, _ := server.store.GeneratedAlerts()
	if len(alerts) != 1 {
		t.Fatal("expected generated alert")
	}

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/alerts/"+alerts[0].ID+"/ack", strings.NewReader(``)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"acknowledged"`) {
		t.Fatalf("ack failed %d: %s", response.Code, response.Body.String())
	}

	view := httptest.NewRecorder()
	server.routes().ServeHTTP(view, httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil))
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), `"state":"acknowledged"`) {
		t.Fatalf("expected persisted acknowledgement in alerts view %d: %s", view.Code, view.Body.String())
	}
	var payload []map[string]any
	if err := json.NewDecoder(view.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
}
