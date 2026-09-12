package main

import (
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
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
	case "docker.stack.rollback":
		stackID, _ := data["stackId"].(string)
		reason, _ := data["reason"].(string)
		s.fireAlertForRule("rule-container", "Stack update rolled back", reason, &model.ResourceRef{Type: "stack", ID: stackID})
	case "docker.stack.update.failed":
		stackID, _ := data["stackId"].(string)
		reason, _ := data["reason"].(string)
		s.fireAlertForRule("rule-container", "Stack update failed", reason, &model.ResourceRef{Type: "stack", ID: stackID})
	}
}

// fireAlertForRule opens a generated alert owned by an alert rule and stamps
// the rule's last-triggered time. It reports whether a new alert was opened
// (deduplicated re-fires return false).
func (s *apiServer) fireAlertForRule(ruleID, title, description string, resource *model.ResourceRef) bool {
	rule, err := s.store.AlertRule(ruleID)
	if err != nil || !rule.Enabled {
		return false
	}
	return s.fireAlertWithSeverity(ruleID, rule.Severity, title, description, resource)
}

// fireAlertWithSeverity opens a generated alert with an explicit severity so
// the same rule can escalate (e.g. SMART wear warning vs failed drive).
func (s *apiServer) fireAlertWithSeverity(ruleID, severity, title, description string, resource *model.ResourceRef) bool {
	rule, err := s.store.AlertRule(ruleID)
	if err != nil || !rule.Enabled {
		return false
	}
	now := time.Now().UTC()
	alert := model.Alert{
		ID:          newID("alert"),
		Severity:    severity,
		Title:       rule.Name + " — " + title,
		Description: description,
		Resource:    resource,
		State:       "firing",
		StartedAt:   now,
	}
	inserted, err := s.store.OpenGeneratedAlert(alert, ruleID)
	if err != nil || !inserted {
		return false
	}
	rule.LastTriggeredAt = &now
	_ = s.store.SaveAlertRule(rule)
	s.publish("alert.created", alert.Severity, &model.ResourceRef{Type: "alert", ID: alert.ID}, map[string]any{
		"alertId": alert.ID, "ruleId": ruleID, "title": alert.Title,
	})
	return true
}

func (s *apiServer) resolveAlertForRule(ruleID, resourceID string) {
	resolved, err := s.store.ResolveGeneratedAlerts(ruleID, resourceID)
	if err != nil || !resolved {
		return
	}
	s.publish("alert.resolved", "info", nil, map[string]any{"ruleId": ruleID, "resourceId": resourceID})
}

// evaluatePeriodicAlerts checks time-based rule conditions. Temperature and
// SMART attributes need disk (SMART) reads, so they are sampled every tenth
// tick (~10 minutes); sync staleness is a cheap meta lookup and runs every
// tick.
func (s *apiServer) evaluatePeriodicAlerts(tick int64) {
	s.evaluateFilesystemCapacity()
	if tick%10 == 0 {
		s.evaluateDiskTemperatures()
		s.evaluateSMARTAlerts()
	}
	s.evaluateSyncStaleness()
}

func (s *apiServer) evaluateFilesystemCapacity() {
	metrics := collector.Metrics()
	s.evaluateFilesystemUsage(metrics.Filesystems)
}

func (s *apiServer) evaluateFilesystemUsage(filesystems []model.FilesystemUsage) {
	for _, filesystem := range filesystems {
		resource := &model.ResourceRef{Type: "filesystem", ID: filesystem.Path}
		if filesystem.State == "critical" || filesystem.State == "warning" {
			severity := "warning"
			if filesystem.State == "critical" {
				severity = "critical"
			}
			s.fireAlertWithSeverity(
				"rule-filesystem",
				severity,
				"Filesystem space is low",
				fmt.Sprintf("%s is %.1f%% used with %d bytes available", filesystem.Path, filesystem.UsedPercent, filesystem.AvailableBytes),
				resource,
			)
			continue
		}
		s.resolveAlertForRule("rule-filesystem", filesystem.Path)
	}
}

// evaluateSMARTAlerts turns SMART attribute movement (reallocated/pending/
// uncorrectable sectors, CRC errors, failed self-assessment) into rule-owned
// alerts so early failure signals notify instead of only colouring the disk
// badge. Alerts resolve when the counters stop indicating trouble.
func (s *apiServer) evaluateSMARTAlerts() {
	disks, err := s.diskFunc()
	if err != nil {
		return
	}
	for _, disk := range disks {
		summary := disk.SMART
		failed := summary.Overall == model.Critical
		degraded := summary.PendingSectors > 0 || summary.UncorrectableSectors > 0 || summary.ReallocatedSectors > 0
		if failed || degraded {
			severity := "warning"
			title := "SMART attributes report disk wear"
			if failed {
				severity = "critical"
				title = "SMART self-assessment failed"
			}
			description := fmt.Sprintf(
				"%s (%s): %d reallocated, %d pending, %d uncorrectable sectors, %d CRC errors",
				disk.Name, disk.Model, summary.ReallocatedSectors, summary.PendingSectors, summary.UncorrectableSectors, summary.CRCErrors,
			)
			if s.fireAlertWithSeverity("rule-smart", severity, title, description, &model.ResourceRef{Type: "disk", ID: disk.ID}) {
				// Publish the dedicated event so notification routing can
				// send SMART movement to channels beyond the web UI.
				s.publish("disk.smart.warning", severity, &model.ResourceRef{Type: "disk", ID: disk.ID}, map[string]any{"diskId": disk.ID, "title": title})
			}
			continue
		}
		s.resolveAlertForRule("rule-smart", disk.ID)
	}
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
