package main

import (
	"context"
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
	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/recovery"
)

var automaticBackupMu sync.Mutex
var automaticBackupPending bool

func (s *apiServer) recoveryKeyString() string {
	if value := os.Getenv("LUMONAS_RECOVERY_KEY"); value != "" {
		return value
	}
	path := envOr("LUMONAS_RECOVERY_KEY_FILE", filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "recovery.key"))
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
	s.recordRequestAudit(r, actor, "backup.destination.save", value.ID, map[string]any{"type": value.Type})
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
	s.recordRequestAudit(r, actor, "backup.destination.delete", id, nil)
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
	status.DestinationHealth = make([]backup.DestinationHealth, 0, len(destinations))
	for _, destination := range destinations {
		health := backup.DestinationHealth{DestinationID: destination.ID, State: "offline"}
		if !destination.Enabled {
			health.State = "disabled"
		}
		status.DestinationHealth = append(status.DestinationHealth, health)
	}
	if len(runs) > 0 {
		status.LatestVerified = runs[0].State == "verified"
		status.LatestGeneration = runs[0].Generation
		if copies, copyErr := s.store.BackupCopies(runs[0].ID); copyErr == nil {
			for _, copy := range copies {
				for index := range status.DestinationHealth {
					if status.DestinationHealth[index].DestinationID == copy.DestinationID {
						if copy.Verified && copy.State == "verified" {
							status.DestinationHealth[index].State = "healthy"
							status.DestinationHealth[index].Verified++
						} else {
							status.DestinationHealth[index].State = "failed"
							status.DestinationHealth[index].LastError = copy.Error
						}
						break
					}
				}
				if copy.State == "verified" && copy.Verified {
					status.HealthyCopies++
				}
			}
		}
	}
	if verifications, verificationErr := s.store.BackupVerifications(500); verificationErr == nil {
		for _, verification := range verifications {
			if verification.VerifiedAt != nil && (status.LastVerification == nil || verification.VerifiedAt.After(*status.LastVerification)) {
				status.LastVerification = verification.VerifiedAt
			}
		}
	}
	var dockerWarnings []string
	status.DockerAppdataCovered, dockerWarnings = s.dockerAppdataCoverage()
	status.Warnings = append(status.Warnings, dockerWarnings...)
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

type backupScheduleRequest struct {
	backup.Schedule
	ExpectedGeneration *int64 `json:"expectedGeneration"`
}

func (s *apiServer) backupSchedule(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	schedule, err := s.store.BackupSchedule()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (s *apiServer) updateBackupSchedule(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	var input backupScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	current, err := s.store.BackupSchedule()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if input.ID == "" {
		input.ID = current.ID
	}
	if input.LastStartedAt == nil {
		input.LastStartedAt = current.LastStartedAt
	}
	if input.NextDueAt == nil {
		input.NextDueAt = current.NextDueAt
	}
	value, err := s.store.SaveBackupSchedule(input.Schedule)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "backup.schedule.update", value.ID, map[string]any{"enabled": value.Enabled, "intervalSeconds": value.IntervalSeconds})
	s.advanceGeneration("backup.schedule.update")
	writeJSON(w, http.StatusOK, value)
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
	s.recordRequestAudit(r, actor, "backup.run.request", run.ID, nil)
	go s.executeBackup(run)
	writeJSON(w, http.StatusAccepted, run)
}

func (s *apiServer) requestAutomaticBackup(trigger string) {
	if os.Getenv("LUMONAS_AUTO_BACKUP_DISABLED") == "true" || s.recoveryKeyString() == "" {
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
	if os.Getenv("LUMONAS_AUTO_BACKUP_DISABLED") == "true" {
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
	bundlePath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
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
	failed, enabled := 0, 0
	for _, destination := range destinations {
		if !destination.Enabled {
			continue
		}
		enabled++
		full, credentials, credentialErr := s.store.BackupDestination(destination.ID, []byte(key))
		copy := backup.Copy{ID: newID("backup-copy"), RunID: run.ID, DestinationID: destination.ID, Object: object, Checksum: digest, Bytes: size, State: "running", CreatedAt: time.Now().UTC()}
		if credentialErr == nil {
			credentialErr = backup.UploadAndVerifyWithRetry(full, credentials, bundlePath, object, digest, size, 3)
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
		verification := backup.Verification{ID: newID("backup-verification"), RunID: run.ID, DestinationID: destination.ID, State: copy.State, Error: copy.Error}
		if copy.Verified {
			now := time.Now().UTC()
			verification.VerifiedAt = &now
		}
		_ = s.store.SaveBackupVerification(verification)
	}
	if enabled == 0 {
		s.finishBackup(run, "failed", errors.New("no enabled backup destination is configured"))
		return
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
	} else if state == "verified" {
		s.publish("recovery.backup.completed", "info", nil, map[string]any{"runId": run.ID})
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
	path := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
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
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		schedule, err := s.store.BackupSchedule()
		if err != nil || !schedule.Enabled {
			continue
		}
		now := time.Now().UTC()
		if schedule.NextDueAt == nil {
			next := now.Add(time.Duration(schedule.IntervalSeconds) * time.Second)
			schedule.NextDueAt = &next
			_, _ = s.store.SaveBackupSchedule(schedule)
			continue
		}
		if now.Before(*schedule.NextDueAt) {
			continue
		}
		schedule.LastStartedAt = &now
		next := now.Add(time.Duration(schedule.IntervalSeconds) * time.Second)
		schedule.NextDueAt = &next
		if _, err := s.store.SaveBackupSchedule(schedule); err == nil {
			s.requestAutomaticBackup("daily")
		}
	}
}

func (s *apiServer) dockerAppdataCoverage() (bool, []string) {
	stacks, err := s.decoratedDockerStacks(context.Background())
	if err != nil {
		return false, []string{"Docker appdata coverage could not be determined"}
	}
	if len(stacks) == 0 {
		return true, nil
	}
	warnings := []string{"Docker appdata content is not included in configuration recovery bundles"}
	for _, stack := range stacks {
		if stack.Recovery == nil || len(stack.Recovery.AppdataPaths) == 0 || stack.Recovery.Strategy == dockerruntime.StrategyNone {
			warnings = append(warnings, stack.Name+" has no configured appdata recovery contract")
			continue
		}
		warnings = append(warnings, stack.Name+" appdata requires a content backup before restore")
	}
	return false, warnings
}
