package main

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
)

func (s *apiServer) fileIntegrityStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("shareId"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	status, err := s.store.IntegrityStatus(share.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.integrityMu.Lock()
	running := s.integrityRunning[share.ID]
	s.integrityMu.Unlock()
	writeJSON(w, 200, map[string]any{"shareId": status.ShareID, "fileCount": status.FileCount, "baselineAt": status.BaselineAt, "report": status.Report, "running": running})
}

func (s *apiServer) startFileIntegrity(w http.ResponseWriter, r *http.Request, baseline bool) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ShareID string `json:"shareId"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if !s.workstationShareAllowed(r, share.ID) {
		writeJSON(w, 403, map[string]string{"error": "workstation backup token is bound to a different share"})
		return
	}
	if !baseline {
		if _, _, err := s.store.IntegrityBaseline(share.ID); err != nil {
			writeJSON(w, 409, map[string]string{"error": "create an integrity baseline before verifying this share"})
			return
		}
	}
	s.integrityMu.Lock()
	if s.integrityRunning == nil {
		s.integrityRunning = map[string]bool{}
	}
	if s.integrityRunning[share.ID] {
		s.integrityMu.Unlock()
		writeJSON(w, 409, map[string]string{"error": "an integrity scan is already running for this share"})
		return
	}
	s.integrityRunning[share.ID] = true
	s.integrityMu.Unlock()
	title := "Verify file integrity"
	eventAction := "file.integrity.verify.request"
	if baseline {
		title = "Create file integrity baseline"
		eventAction = "file.integrity.baseline.request"
	}
	job := s.queueFileJob(actor, requestCorrelationID(r), title, share.ID, func() (map[string]any, error) {
		defer func() { s.integrityMu.Lock(); delete(s.integrityRunning, share.ID); s.integrityMu.Unlock() }()
		manifest, scanErr := fileops.BuildIntegrityManifest(share.Path)
		if scanErr != nil {
			return nil, scanErr
		}
		now := time.Now().UTC()
		if baseline {
			records := make([]store.IntegrityRecord, 0, len(manifest))
			for _, entry := range manifest {
				records = append(records, store.IntegrityRecord{Path: entry.Path, SHA256: entry.SHA256, SizeBytes: entry.SizeBytes, ModifiedAt: entry.ModifiedAt})
			}
			if err := s.store.ReplaceIntegrityBaseline(share.ID, records, now); err != nil {
				return nil, err
			}
			return map[string]any{"shareId": share.ID, "fileCount": len(records), "baselineAt": now}, nil
		}
		baselineAt, previous, baselineErr := s.store.IntegrityBaseline(share.ID)
		if baselineErr != nil {
			return nil, baselineErr
		}
		current := map[string]fileops.IntegrityFile{}
		for _, entry := range manifest {
			current[entry.Path] = entry
		}
		report := store.IntegrityReport{BaselineAt: baselineAt, VerifiedAt: now, Changed: []string{}, Missing: []string{}, Added: []string{}}
		for name, old := range previous {
			fresh, exists := current[name]
			if !exists {
				report.MissingCount++
				if len(report.Missing) < 100 {
					report.Missing = append(report.Missing, name)
				}
				continue
			}
			if old.SHA256 != fresh.SHA256 || old.SizeBytes != fresh.SizeBytes {
				report.ChangedCount++
				if len(report.Changed) < 100 {
					report.Changed = append(report.Changed, name)
				}
				continue
			}
			report.Unchanged++
		}
		for name := range current {
			if _, exists := previous[name]; !exists {
				report.AddedCount++
				if len(report.Added) < 100 {
					report.Added = append(report.Added, name)
				}
			}
		}
		if err := s.store.SaveIntegrityReport(share.ID, report); err != nil {
			return nil, err
		}
		return map[string]any{"shareId": share.ID, "unchanged": report.Unchanged, "changed": report.ChangedCount, "missing": report.MissingCount, "added": report.AddedCount, "samples": report}, nil
	})
	s.recordRequestAudit(r, actor, eventAction, share.ID, map[string]any{"jobId": job.ID})
	s.publishActor(actor, "file.integrity.scan.queued", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"jobId": job.ID, "baseline": baseline})
	if job.State == "failed" {
		s.integrityMu.Lock()
		delete(s.integrityRunning, share.ID)
		s.integrityMu.Unlock()
	}
	writeJSON(w, http.StatusAccepted, job)
}
