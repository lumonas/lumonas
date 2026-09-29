package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/lumonas/lumonas/internal/capacity"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
)

func (s *apiServer) capacityForecast(w http.ResponseWriter, r *http.Request) {
	windowDays := 30
	if value := r.URL.Query().Get("days"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 366 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be between 1 and 366"})
			return
		}
		windowDays = parsed
	}
	since := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour)
	resourceID := r.URL.Query().Get("resourceId")
	resources := []string{resourceID}
	if resourceID == "" {
		var err error
		resources, err = s.store.CapacityResources(since)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	result := make([]capacity.Forecast, 0, len(resources))
	for _, resource := range resources {
		snapshots, err := s.store.CapacitySnapshots(resource, since, 366)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result = append(result, capacity.Build(resource, snapshots, time.Now().UTC()))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) capacityThresholds(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.CapacityThresholds()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) setCapacityThreshold(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ResourceID string `json:"resourceId"`
		Percent    int    `json:"thresholdPercent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.store.SetCapacityThreshold(input.ResourceID, input.Percent); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	_ = s.store.SaveAudit(store.AuditEntry{Actor: actor, Action: "capacity.threshold.update", Outcome: "recorded", ResourceType: "storage", ResourceID: input.ResourceID, Metadata: map[string]any{"thresholdPercent": input.Percent}})
	writeJSON(w, http.StatusOK, store.CapacityThreshold{ResourceID: input.ResourceID, Percent: input.Percent, UpdatedAt: time.Now().UTC()})
}

func (s *apiServer) capacityThresholdAlerts(now time.Time) []model.Alert {
	thresholds, err := s.store.CapacityThresholds()
	if err != nil {
		return nil
	}
	alerts := make([]model.Alert, 0)
	for _, threshold := range thresholds {
		samples, err := s.store.CapacitySnapshots(threshold.ResourceID, now.Add(-48*time.Hour), 1)
		if err != nil || len(samples) == 0 || samples[0].TotalBytes == 0 {
			continue
		}
		latest := samples[0]
		used := int(float64(latest.UsedBytes) * 100 / float64(latest.TotalBytes))
		if used >= threshold.Percent {
			digest := sha256.Sum256([]byte(threshold.ResourceID))
			alerts = append(alerts, model.Alert{ID: "capacity-threshold-" + hex.EncodeToString(digest[:8]), Severity: "warning", Title: "Storage capacity is above its threshold", Description: fmt.Sprintf("%s is %d%% used; configured threshold is %d%%", threshold.ResourceID, used, threshold.Percent), Resource: &model.ResourceRef{Type: "storage", ID: threshold.ResourceID}, State: "firing", StartedAt: latest.CapturedAt})
		}
	}
	return alerts
}
