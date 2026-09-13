package main

import (
	"net/http"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/diagnostics"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
)

func (s *apiServer) supportBundle(w http.ResponseWriter, _ *http.Request) {
	collectionErrors := make(map[string]string)
	serverID, serverIDOK := s.store.Meta("nas_uuid")
	if !serverIDOK {
		collectionErrors["serverIdentity"] = "unavailable"
	}
	disks, diskErr := s.diskFunc()
	if diskErr != nil {
		disks = []model.Disk{}
		collectionErrors["disks"] = "unavailable"
	}
	events, eventErr := s.store.Events(500)
	if eventErr != nil {
		events = []model.Event{}
		collectionErrors["events"] = "unavailable"
	}
	audit, auditErr := s.store.Audit(500)
	if auditErr != nil {
		audit = []store.AuditEntry{}
		collectionErrors["audit"] = "unavailable"
	}
	metricHistory, metricHistoryErr := s.store.SystemMetricSamples(time.Now().UTC().Add(-24*time.Hour), 1440)
	if metricHistoryErr != nil {
		metricHistory = []model.SystemMetricSample{}
		collectionErrors["metricHistory"] = "unavailable"
	}
	server := map[string]any{
		"nasUuid":  serverID,
		"hostname": collector.Hostname(),
		"version":  s.version,
		"health":   s.serverHealth(),
	}
	if len(collectionErrors) > 0 {
		server["collectionErrors"] = collectionErrors
	}
	entries := map[string][]byte{}
	for name, value := range map[string]any{
		"server.json":          server,
		"metrics.json":         collector.Metrics(),
		"metrics-history.json": metricHistory,
		"disks.json":           disks,
		"events.json":          events,
		"audit.json":           audit,
	} {
		data, err := diagnostics.MarshalJSON(value)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "support bundle serialization failed"})
			return
		}
		entries[name] = data
	}
	bundle, err := diagnostics.CreateBundle(entries)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "support bundle creation failed"})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=lumonas-support-"+time.Now().UTC().Format("20060102T150405Z")+".zip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bundle)
}
