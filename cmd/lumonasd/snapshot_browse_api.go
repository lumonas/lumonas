package main

import (
	"context"
	"net/http"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

// storageSnapshotFiles lists one directory inside a browsable snapshot.
// Only btrfs snapshots expose paths on the filesystem; zfs snapshot content
// is documented as not browsable.
func (s *apiServer) storageSnapshotFiles(w http.ResponseWriter, r *http.Request, id string) {
	record, found, err := s.store.StorageSnapshot(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
		return
	}
	if record.Kind != "btrfs" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "only btrfs snapshots can be browsed"})
		return
	}
	subpath := r.URL.Query().Get("path")
	request := privileged.Request{
		Operation: "snapshot.browse",
		PlanHash:  "browse-" + id,
		RequestedState: map[string]any{
			"kind":    record.Kind,
			"source":  record.Source,
			"name":    record.Name,
			"subpath": subpath,
		},
		Confirmed: true,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := s.executePrivileged(ctx, request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	entries, total := snapshotEntries(result)
	writeJSON(w, http.StatusOK, map[string]any{"path": subpath, "entries": entries, "total": total})
}

// snapshotEntries decodes the broker payload. The brokerExec test seam
// returns no payload, which yields an empty listing rather than an error.
func snapshotEntries(result privileged.Response) ([]model.SnapshotEntry, int) {
	entries := make([]model.SnapshotEntry, 0)
	data, ok := result.Data.(map[string]any)
	if !ok {
		return entries, 0
	}
	total := len(entries)
	if value, ok := data["total"].(float64); ok && value >= 0 {
		total = int(value)
	}
	raw, ok := data["entries"].([]any)
	if !ok {
		return entries, total
	}
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		decoded := model.SnapshotEntry{}
		if name, ok := entry["name"].(string); ok {
			decoded.Name = name
		} else {
			continue
		}
		if size, ok := entry["sizeBytes"].(float64); ok {
			decoded.SizeBytes = int64(size)
		}
		decoded.Directory, _ = entry["directory"].(bool)
		if modified, ok := entry["modifiedAt"].(string); ok {
			if parsed, parseErr := time.Parse(time.RFC3339, modified); parseErr == nil {
				decoded.ModifiedAt = parsed
			}
		}
		entries = append(entries, decoded)
	}
	return entries, total
}
