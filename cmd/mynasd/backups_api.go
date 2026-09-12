package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/recovery"
)

var automaticBackupMu sync.Mutex
var automaticBackupPending bool

func (s *apiServer) recoveryKeyString() string {
	if value := os.Getenv("MYNAS_RECOVERY_KEY"); value != "" {
		return value
	}
	path := envOr("MYNAS_RECOVERY_KEY_FILE", filepath.Join(envOr("MYNAS_RECOVERY_DIR", "/var/lib/mynas/recovery"), "recovery.key"))
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		return ""
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func (s *apiServer) backupActor(w http.ResponseWriter, r *http.Request, mutate bool) (string, bool) {
	return s.identityActor(w, r, mutate)
}

func (s *apiServer) listBackupDestinations(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	values, err := s.store.ListBackupDestinations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) saveBackupDestination(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is not configured"})
		return
	}
	var input struct {
		backup.Destination
		Credentials backup.Credentials `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.ID == "" {
		input.ID = newID("backup")
	}
	if input.Retention == (backup.RetentionPolicy{}) {
		input.Retention = backup.DefaultRetention()
	}
	value, err := s.store.SaveBackupDestination(input.Destination, input.Credentials, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "backup.destination.save", value.ID, map[string]any{"type": value.Type})
	s.advanceGeneration("backup.destination.save")
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) deleteBackupDestination(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteBackupDestination(id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "backup destination not found"})
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "backup destination not found"})
		}
		return
	}
	s.recordIdentityAudit(actor, "backup.destination.delete", id, nil)
	s.advanceGeneration("backup.destination.delete")
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) backupStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	destinations, err := s.store.ListBackupDestinations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	runs, err := s.store.BackupRuns(1)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	status := backup.Readiness{RecoveryKey: s.recoveryKeyString() != "", DestinationCount: len(destinations), CheckedAt: time.Now().UTC()}
	status.Configured = status.RecoveryKey && len(destinations) > 0
	if len(runs) > 0 {
		status.LatestVerified = runs[0].State == "verified"
		status.LatestGeneration = runs[0].Generation
		if copies, copyErr := s.store.BackupCopies(runs[0].ID); copyErr == nil {
			for _, copy := range copies {
				if copy.State == "verified" && copy.Verified {
					status.HealthyCopies++
				}
			}
		}
	}
	if !status.RecoveryKey {
		status.Warnings = append(status.Warnings, "recovery key is not configured")
	}
	if len(destinations) == 0 {
		status.Warnings = append(status.Warnings, "no backup destination is configured")
	}
	if len(runs) == 0 {
		status.Warnings = append(status.Warnings, "no automated backup has completed")
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *apiServer) listBackupRuns(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	runs, err := s.store.BackupRuns(50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type result struct {
		Run    backup.Run    `json:"run"`
		Copies []backup.Copy `json:"copies"`
	}
	values := make([]result, 0, len(runs))
	for _, run := range runs {
		copies, copyErr := s.store.BackupCopies(run.ID)
		if copyErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": copyErr.Error()})
			return
		}
		values = append(values, result{run, copies})
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) runBackupNow(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	run := backup.Run{ID: newID("backup-run"), Trigger: "manual", Generation: s.currentGeneration(), State: "queued", StartedAt: time.Now().UTC()}
	if err := s.store.SaveBackupRun(run); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "backup.run.request", run.ID, nil)
	go s.executeBackup(run)
	writeJSON(w, http.StatusAccepted, run)
}

func (s *apiServer) requestAutomaticBackup(trigger string) {
	if os.Getenv("MYNAS_AUTO_BACKUP_DISABLED") == "true" || s.recoveryKeyString() == "" {
		return
	}
	automaticBackupMu.Lock()
	if automaticBackupPending {
		automaticBackupMu.Unlock()
		return
	}
	automaticBackupPending = true
	automaticBackupMu.Unlock()
	go func() {
		defer func() { automaticBackupMu.Lock(); automaticBackupPending = false; automaticBackupMu.Unlock() }()
		run := backup.Run{ID: newID("backup-run"), Trigger: trigger, Generation: s.currentGeneration(), State: "queued", StartedAt: time.Now().UTC()}
		if err := s.store.SaveBackupRun(run); err == nil {
			s.executeBackup(run)
		}
	}()
}

func (s *apiServer) executeBackup(run backup.Run) {
	run.State = "running"
	_ = s.store.SaveBackupRun(run)
	if os.Getenv("MYNAS_AUTO_BACKUP_DISABLED") == "true" {
		s.finishBackup(run, "skipped", errors.New("automatic backups are disabled"))
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		s.finishBackup(run, "failed", errors.New("recovery key is not configured"))
		return
	}
	recorder := httptest.NewRecorder()
	s.recoveryExport(recorder)
	if recorder.Code < 200 || recorder.Code >= 300 {
		s.finishBackup(run, "failed", fmt.Errorf("recovery export failed: %s", strings.TrimSpace(recorder.Body.String())))
		return
	}
	bundlePath := filepath.Join(envOr("MYNAS_RECOVERY_DIR", "/var/lib/mynas/recovery"), "latest.mrb")
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		s.finishBackup(run, "failed", err)
		return
	}
	manifest, err := recovery.Verify(bundle, []byte(key))
	if err != nil {
		s.finishBackup(run, "failed", fmt.Errorf("bundle verification failed: %w", err))
		return
	}
	digest, size, err := backup.SHA256File(bundlePath)
	if err != nil {
		s.finishBackup(run, "failed", err)
		return
	}
	run.Generation, run.BundlePath, run.Checksum, run.Bytes = manifest.Generation, bundlePath, digest, size
	if err := s.store.SaveBackupRun(run); err != nil {
		return
	}
	destinations, err := s.store.ListBackupDestinations()
	if err != nil {
		s.finishBackup(run, "failed", err)
		return
	}
	object := backup.ObjectName(manifest.Generation, manifest.CreatedAt)
	failed := 0
	for _, destination := range destinations {
		if !destination.Enabled {
			continue
		}
		full, credentials, credentialErr := s.store.BackupDestination(destination.ID, []byte(key))
		copy := backup.Copy{ID: newID("backup-copy"), RunID: run.ID, DestinationID: destination.ID, Object: object, Checksum: digest, Bytes: size, State: "running", CreatedAt: time.Now().UTC()}
		if credentialErr == nil {
			credentialErr = backup.UploadWithTimeout(full, credentials, bundlePath, object)
		}
		if credentialErr != nil {
			copy.State, copy.Error = "failed", credentialErr.Error()
			failed++
		} else {
			copy.State, copy.Verified = "verified", true
			now := time.Now().UTC()
			copy.FinishedAt = &now
		}
		_ = s.store.SaveBackupCopy(copy)
	}
	if failed > 0 {
		s.finishBackup(run, "failed", fmt.Errorf("%d backup destination(s) failed", failed))
		return
	}
	s.finishBackup(run, "verified", nil)
	s.pruneBackupCopies(destinations, []byte(key))
	s.publish("recovery.backup.verified", "info", nil, map[string]any{"runId": run.ID, "generation": run.Generation, "destinations": len(destinations)})
}

func (s *apiServer) pruneBackupCopies(destinations []backup.Destination, key []byte) {
	runs, err := s.store.BackupRuns(500)
	if err != nil {
		return
	}
	for _, destination := range destinations {
		if !destination.Enabled {
			continue
		}
		retained := make(map[string]bool)
		for _, run := range backup.RetainedRuns(runs, destination.Retention) {
			retained[run.ID] = true
		}
		copies, copyErr := s.store.BackupCopiesForDestination(destination.ID)
		if copyErr != nil {
			continue
		}
		full, credentials, credentialErr := s.store.BackupDestination(destination.ID, key)
		if credentialErr != nil {
			continue
		}
		for _, copy := range copies {
			if retained[copy.RunID] || copy.State != "verified" {
				continue
			}
			if err := backup.DeleteWithTimeout(full, credentials, copy.Object); err != nil {
				continue
			}
			_ = s.store.DeleteBackupCopy(copy.ID)
		}
	}
}

func (s *apiServer) finishBackup(run backup.Run, state string, failure error) {
	run.State = state
	now := time.Now().UTC()
	run.FinishedAt = &now
	if failure != nil {
		run.Error = failure.Error()
		s.publish("recovery.backup.failed", "warning", nil, map[string]any{"runId": run.ID, "error": run.Error})
	}
	_ = s.store.SaveBackupRun(run)
}

func (s *apiServer) verifyBackupNow(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, true); !ok {
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is not configured"})
		return
	}
	path := filepath.Join(envOr("MYNAS_RECOVERY_DIR", "/var/lib/mynas/recovery"), "latest.mrb")
	bundle, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "latest recovery bundle not found"})
		return
	}
	manifest, err := recovery.Verify(bundle, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "backup verification failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verified": true, "manifest": manifest})
}

func (s *apiServer) backupLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.requestAutomaticBackup("daily")
	}
}
