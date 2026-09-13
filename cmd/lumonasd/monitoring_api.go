package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path"

	"github.com/lumonas/lumonas/internal/monitoring"
)

func (s *apiServer) alertRules(w http.ResponseWriter) {
	rules, err := s.store.AlertRules()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *apiServer) updateAlertRule(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	rule, err := s.store.AlertRule(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert rule not found"})
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled is required"})
		return
	}
	rule.Enabled = *input.Enabled
	if err := s.store.SaveAlertRule(rule); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.advanceGeneration("alert-rule.update")
	s.publishActor(actor, "alert.rule.updated", "info", nil, map[string]any{"ruleId": id, "enabled": rule.Enabled})
	writeJSON(w, http.StatusOK, rule)
}

func (s *apiServer) notificationChannels(w http.ResponseWriter) {
	channels := []monitoring.NotificationChannel{{ID: "ch-web", Type: "web", Label: "Web UI", Configured: true, Enabled: true}}
	if target := os.Getenv("LUMONAS_NOTIFY_NTFY_URL"); target != "" {
		channels = append(channels, monitoring.NotificationChannel{ID: "ch-ntfy", Type: "ntfy", Label: "ntfy", Target: "configured", Configured: true, Enabled: true})
	}
	if os.Getenv("LUMONAS_NOTIFY_WEBHOOK_URL") != "" {
		channels = append(channels, monitoring.NotificationChannel{ID: "ch-webhook", Type: "webhook", Label: "Webhook", Target: "configured", Configured: true, Enabled: true})
	}
	writeJSON(w, http.StatusOK, channels)
}

func alertRuleID(endpoint string) string {
	return path.Base(endpoint)
}
