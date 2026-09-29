package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
)

func (s *apiServer) recordSMARTSamples() {
	disks, err := s.diskFunc()
	if err != nil {
		return
	}
	for _, disk := range disks {
		if err := s.store.SaveSMARTSample(model.SMARTSample{DiskID: disk.ID, CapturedAt: time.Now().UTC(), Summary: disk.SMART, TemperatureC: disk.Temperature}); err != nil && s.log != nil {
			s.log.Warn("SMART history persistence failed", "disk", disk.ID, "error", err)
		}
	}
	_ = s.store.PruneSMARTSamples(time.Now().UTC().Add(-365 * 24 * time.Hour))
}

func (s *apiServer) smartHistoryLoop() {
	interval := 15 * time.Minute
	if raw := strings.TrimSpace(envOr("LUMONAS_SMART_HISTORY_INTERVAL", "")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed >= time.Minute {
			interval = parsed
		}
	}
	s.recordSMARTSamples()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		s.recordSMARTSamples()
	}
}

func (s *apiServer) smartHistory(w http.ResponseWriter, r *http.Request, diskID string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 365 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be between 1 and 365"})
			return
		}
		days = parsed
	}
	samples, err := s.store.SMARTSamples(diskID, time.Now().UTC().Add(-time.Duration(days)*24*time.Hour), 2000)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.SMARTHistory{DiskID: diskID, Samples: samples, Trend: collector.SMARTTrend(samples)})
}
