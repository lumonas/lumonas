package main

import (
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

const syncStaleThreshold = 48 * time.Hour

// evaluateEventAlert fires or resolves rule-based alerts from published events.
// It is invoked from publish(), so it must never publish the same event kind
// it reacts to (preventing recursion).
func (s *apiServer) evaluateEventAlert(kind string, data map[string]any) {
	switch kind {
	case "recovery.backup.failed":
		message, _ := data["error"].(string)
		s.fireAlertForRule("rule-backup", "Backup job failed", message, nil)
	case "recovery.backup.completed":
		s.resolveAlertForRule("rule-backup", "")
	case "snapraid.sync.failed", "snapraid.scrub.failed":
		message, _ := data["error"].(string)
		s.fireAlertForRule("rule-sync", "SnapRAID operation failed", message, &model.ResourceRef{Type: "protection", ID: "protection"})
	case "snapraid.sync.completed":
		s.resolveAlertForRule("rule-sync", "protection")
	}
}

// fireAlertForRule opens a generated alert owned by an alert rule and stamps
// the rule's last-triggered time.
func (s *apiServer) fireAlertForRule(ruleID, title, description string, resource *model.ResourceRef) {
	rule, err := s.store.AlertRule(ruleID)
	if err != nil || !rule.Enabled {
		return
	}
	now := time.Now().UTC()
	alert := model.Alert{
		ID:          newID("alert"),
		Severity:    rule.Severity,
		Title:       rule.Name + " — " + title,
		Description: description,
		Resource:    resource,
		State:       "firing",
		StartedAt:   now,
	}
	inserted, err := s.store.OpenGeneratedAlert(alert, ruleID)
	if err != nil || !inserted {
		return
	}
	rule.LastTriggeredAt = &now
	_ = s.store.SaveAlertRule(rule)
	s.publish("alert.created", alert.Severity, &model.ResourceRef{Type: "alert", ID: alert.ID}, map[string]any{
		"alertId": alert.ID, "ruleId": ruleID, "title": alert.Title,
	})
}

func (s *apiServer) resolveAlertForRule(ruleID, resourceID string) {
	resolved, err := s.store.ResolveGeneratedAlerts(ruleID, resourceID)
	if err != nil || !resolved {
		return
	}
	s.publish("alert.resolved", "info", nil, map[string]any{"ruleId": ruleID, "resourceId": resourceID})
}

// evaluatePeriodicAlerts checks time-based rule conditions. Temperature needs
// SMART reads, so it is sampled every tenth tick (~10 minutes); sync staleness
// is a cheap meta lookup and runs every tick.
func (s *apiServer) evaluatePeriodicAlerts(tick int64) {
	if tick%10 == 0 {
		s.evaluateDiskTemperatures()
	}
	s.evaluateSyncStaleness()
}

func (s *apiServer) evaluateDiskTemperatures() {
	disks, err := s.diskFunc()
	if err != nil {
		return
	}
	for _, disk := range disks {
		if disk.Temperature == nil {
			continue
		}
		if *disk.Temperature > 45 {
			s.fireAlertForRule(
				"rule-temp",
				"Disk temperature high",
				fmt.Sprintf("%s (%s) is running at %.0f°C", disk.Name, disk.Model, *disk.Temperature),
				&model.ResourceRef{Type: "disk", ID: disk.ID},
			)
			continue
		}
		s.resolveAlertForRule("rule-temp", disk.ID)
	}
}

func (s *apiServer) evaluateSyncStaleness() {
	raw, ok := s.store.Meta("snapraid_last_sync_at")
	if !ok || raw == "" {
		// Never synced (or pre-onboarding): onboarding queues the first sync.
		return
	}
	lastSync, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return
	}
	if time.Since(lastSync) > syncStaleThreshold {
		s.fireAlertForRule(
			"rule-sync",
			"SnapRAID sync stale",
			fmt.Sprintf("No successful sync in %s — recent changes are not protected.", time.Since(lastSync).Round(time.Hour)),
			&model.ResourceRef{Type: "protection", ID: "protection"},
		)
		return
	}
	s.resolveAlertForRule("rule-sync", "protection")
}
