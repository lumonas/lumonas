package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/notify"
)

type notificationChannelRequest struct {
	notify.Channel
	Credentials notify.Credentials `json:"credentials"`
}

type notificationFailureState struct {
	Failures        int
	SuppressedUntil time.Time
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
	s.recordRequestAudit(r, actor, "notification.channel.save", value.ID, map[string]any{"type": value.Type})
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
	s.recordRequestAudit(r, actor, "notification.channel.update", id, map[string]any{"type": value.Type})
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
	s.recordRequestAudit(r, actor, "notification.channel.delete", id, nil)
	s.advanceGeneration("notification.channel.delete")
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) testNotificationChannel(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	key := []byte(s.recoveryKeyString())
	channel, credentials, err := s.store.NotificationChannel(id, key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "notification channel not found or credentials unavailable"})
		return
	}
	message := notify.Message{Title: "LumoNAS test notification", Body: "This is a test notification from LumoNAS.", Severity: "info"}
	if r.Body != nil {
		var requested notify.Message
		if err := json.NewDecoder(r.Body).Decode(&requested); err == nil {
			if strings.TrimSpace(requested.Title) != "" {
				message.Title = requested.Title
			}
			if strings.TrimSpace(requested.Body) != "" {
				message.Body = requested.Body
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := notify.SendWithRetry(ctx, 3, func(ctx context.Context) error {
		return notify.SendChannel(ctx, s.notificationClient, channel, credentials, message)
	}); err != nil {
		s.recordRequestAudit(r, actor, "notification.channel.test", id, map[string]any{"outcome": "failed"})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "notification test delivery failed"})
		return
	}
	s.recordRequestAudit(r, actor, "notification.channel.test", id, map[string]any{"outcome": "sent"})
	writeJSON(w, http.StatusOK, map[string]any{"sent": true, "channelId": id})
}

func (s *apiServer) listNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	deliveries, err := s.store.NotificationDeliveries(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, deliveries)
}

func (s *apiServer) notificationDeliveryAllowed(channelID, eventType string) bool {
	key := channelID + "\x00" + eventType
	now := time.Now().UTC()
	s.notificationMu.Lock()
	state, ok := s.notificationFailures[key]
	s.notificationMu.Unlock()
	if !ok {
		failures, suppressedUntil, found, err := s.store.NotificationFailure(channelID, eventType)
		if err == nil && found {
			state = notificationFailureState{Failures: failures, SuppressedUntil: suppressedUntil}
			s.notificationMu.Lock()
			if s.notificationFailures == nil {
				s.notificationFailures = make(map[string]notificationFailureState)
			}
			s.notificationFailures[key] = state
			s.notificationMu.Unlock()
		}
	}
	if !state.SuppressedUntil.IsZero() {
		if now.Before(state.SuppressedUntil) {
			return false
		}
		// Start a fresh failure window after suppression expires. Otherwise
		// the next isolated failure would immediately re-suppress the channel.
		s.notificationMu.Lock()
		delete(s.notificationFailures, key)
		s.notificationMu.Unlock()
		_ = s.store.ClearNotificationFailure(channelID, eventType)
	}
	return true
}

func (s *apiServer) recordNotificationDeliveryFailure(channelID, eventType string, failed bool) {
	key := channelID + "\x00" + eventType
	s.notificationMu.Lock()
	if s.notificationFailures == nil {
		s.notificationFailures = make(map[string]notificationFailureState)
	}
	if !failed {
		delete(s.notificationFailures, key)
		s.notificationMu.Unlock()
		if err := s.store.ClearNotificationFailure(channelID, eventType); err != nil && s.log != nil {
			s.log.Warn("notification failure state clear failed", "channel", channelID, "event", eventType, "error", err)
		}
		return
	}
	state := s.notificationFailures[key]
	state.Failures++
	if state.Failures >= 3 {
		state.SuppressedUntil = time.Now().UTC().Add(5 * time.Minute)
	}
	s.notificationFailures[key] = state
	s.notificationMu.Unlock()
	if err := s.store.SaveNotificationFailure(channelID, eventType, state.Failures, state.SuppressedUntil); err != nil && s.log != nil {
		s.log.Warn("notification failure state persist failed", "channel", channelID, "event", eventType, "error", err)
	}
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
	// Creation always starts enabled — bool zero-value would otherwise
	// create silently inactive rules; disable via PATCH after creation.
	rule.Enabled = true
	if strings.TrimSpace(rule.Name) == "" || strings.TrimSpace(rule.Condition) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "rule name and condition are required"})
		return
	}
	if err := s.store.SaveAlertRule(rule); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "notification.rule.save", rule.ID, nil)
	s.advanceGeneration("notification.rule.save")
	writeJSON(w, http.StatusOK, rule)
}
