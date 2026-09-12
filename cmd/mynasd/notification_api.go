package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/notify"
)

type notificationChannelRequest struct {
	notify.Channel
	Credentials notify.Credentials `json:"credentials"`
}

func (s *apiServer) saveNotificationChannel(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input notificationChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.ID == "" {
		input.ID = newID("notify")
	}
	key := s.recoveryKeyString()
	if key == "" && notify.HasCredentials(input.Credentials) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is required to encrypt notification credentials"})
		return
	}
	value, err := s.store.SaveNotificationChannel(input.Channel, input.Credentials, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "notification.channel.save", value.ID, map[string]any{"type": value.Type})
	s.advanceGeneration("notification.channel.save")
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.ListNotificationChannels()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	result := make([]monitoring.NotificationChannel, 0, len(values)+1)
	result = append(result, monitoring.NotificationChannel{ID: "ch-web", Type: "web", Label: "Web UI", Configured: true, Enabled: true})
	for _, value := range values {
		result = append(result, monitoring.NotificationChannel{ID: value.ID, Type: value.Type, Label: value.Label, Target: value.Target, Configured: value.Configured, Enabled: value.Enabled})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) updateNotificationChannel(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input notificationChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	input.ID = id
	key := s.recoveryKeyString()
	if !notify.HasCredentials(input.Credentials) {
		_, existing, existingErr := s.store.NotificationChannel(id, []byte(key))
		if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "existing notification credentials could not be read"})
			return
		}
		input.Credentials = existing
	}
	value, err := s.store.SaveNotificationChannel(input.Channel, input.Credentials, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "notification.channel.update", id, map[string]any{"type": value.Type})
	s.advanceGeneration("notification.channel.update")
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) deleteNotificationChannel(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteNotificationChannel(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "notification channel not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "notification.channel.delete", id, nil)
	s.advanceGeneration("notification.channel.delete")
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) saveNotificationRule(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var rule monitoring.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if rule.ID == "" {
		rule.ID = newID("rule")
	}
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	if len(rule.Routes) == 0 {
		rule.Routes = []string{"web"}
	}
	if strings.TrimSpace(rule.Name) == "" || strings.TrimSpace(rule.Condition) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "rule name and condition are required"})
		return
	}
	if err := s.store.SaveAlertRule(rule); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "notification.rule.save", rule.ID, nil)
	s.advanceGeneration("notification.rule.save")
	writeJSON(w, http.StatusOK, rule)
}
