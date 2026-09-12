package main

import (
	"net/http"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/diagnostics"
)

func (s *apiServer) supportBundle(w http.ResponseWriter, _ *http.Request) {
	serverID, _ := s.store.Meta("nas_uuid")
	disks, _ := s.diskFunc()
	events, _ := s.store.Events(500)
	audit, _ := s.store.Audit(500)
	server := map[string]any{
		"nasUuid":  serverID,
		"hostname": collector.Hostname(),
		"version":  s.version,
		"health":   s.serverHealth(),
	}
	entries := map[string][]byte{}
	for name, value := range map[string]any{
		"server.json":  server,
		"metrics.json": collector.Metrics(),
		"disks.json":   disks,
		"events.json":  events,
		"audit.json":   audit,
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
