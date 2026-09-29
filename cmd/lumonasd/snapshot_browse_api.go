package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
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
	if !s.snapshotReadAllowed(w, r, record.Source) {
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

// storageSnapshotCompare produces a bounded metadata diff for a Btrfs
// snapshot. It is intentionally read-only and uses the privileged storage
// worker so private share files remain comparable without widening daemon
// filesystem permissions.
func (s *apiServer) storageSnapshotCompare(w http.ResponseWriter, r *http.Request, id string) {
	record, found, err := s.store.StorageSnapshot(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
		return
	}
	if !s.snapshotReadAllowed(w, r, record.Source) {
		return
	}
	if record.Kind != "btrfs" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "only btrfs snapshots can be compared with the current share"})
		return
	}
	request := privileged.Request{
		Operation: "snapshot.compare",
		PlanHash:  "compare-" + id,
		RequestedState: map[string]any{
			"kind":   record.Kind,
			"source": record.Source,
			"name":   record.Name,
		},
		Confirmed: true,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := s.executePrivileged(ctx, request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "snapshot comparison service is unavailable"})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	writeJSON(w, http.StatusOK, result.Data)
}

// restoreStorageSnapshotEntries copies selected entries from a Btrfs snapshot
// back into its owning managed share. It never overwrites existing files.
func (s *apiServer) restoreStorageSnapshotEntries(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		ShareID                 string   `json:"shareId"`
		SnapshotPath            string   `json:"snapshotPath"`
		TargetPath              string   `json:"targetPath"`
		CreateTargetDirectories bool     `json:"createTargetDirectories"`
		Names                   []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(input.Names) == 0 || len(input.Names) > 100 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "select between 1 and 100 snapshot entries"})
		return
	}
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
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "only btrfs snapshots can be restored through Files"})
		return
	}
	if err := storage.ValidateSnapshotSource(storage.SnapshotBtrfs, record.Source); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot source is invalid"})
		return
	}
	if err := storage.ValidateSnapshotName(record.Name); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot name is invalid"})
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if filepath.Clean(share.Path) != filepath.Clean(record.Source) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "snapshot does not belong to this managed share"})
		return
	}
	actor, ok := s.filePortalRestoreActor(w, r, share.ID)
	if !ok {
		return
	}
	for _, name := range input.Names {
		if strings.TrimSpace(name) != name || name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot entry name is invalid"})
			return
		}
	}
	sourceRoot := record.Source + ".snapshots/" + record.Name
	transfer := fileops.TransferInput{
		SourceRoot: sourceRoot, SourcePath: input.SnapshotPath, Names: input.Names,
		TargetRoot: share.Path, TargetPath: input.TargetPath, Operation: "copy",
	}
	if input.CreateTargetDirectories {
		if err := fileops.EnsureDirectory(share.Path, input.TargetPath); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "recovery folder could not be created safely: " + err.Error()})
			return
		}
	}
	if conflicts, err := fileops.Conflicts(transfer); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot restore path is unavailable: " + err.Error()})
		return
	} else if len(conflicts) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "restore would overwrite existing entries", "conflicts": conflicts})
		return
	}
	job := s.queueFileJob(actor, requestCorrelationID(r), "Restore snapshot entries", share.ID, func() (map[string]any, error) {
		count, transferErr := fileops.Transfer(transfer)
		if transferErr != nil {
			return nil, transferErr
		}
		return map[string]any{"restored": count}, nil
	})
	if job.State == "failed" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("could not queue snapshot restore: %s", job.Error)})
		return
	}
	s.recordRequestAudit(r, actor, "storage.snapshot.restore", id, map[string]any{"jobId": job.ID, "shareId": share.ID, "names": input.Names})
	s.publishActor(actor, "storage.snapshot.restore.queued", "info", &model.ResourceRef{Type: "job", ID: job.ID}, nil)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": job.ID, "state": job.State, "restoring": len(input.Names)})
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
