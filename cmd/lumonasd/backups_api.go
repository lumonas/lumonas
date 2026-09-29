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
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/store"
	"github.com/lumonas/lumonas/internal/virtualization"
)

const interruptedBackupReason = "daemon restarted before backup completed"

type vmBackupManifest struct {
	RunID string               `json:"runId"`
	VMs   []vmBackupManifestVM `json:"virtualMachines"`
}

type vmBackupManifestVM struct {
	Name               string                  `json:"name"`
	Disks              []vmBackupManifestDisk  `json:"disks"`
	DefinitionChecksum string                  `json:"definitionChecksum"`
	DefinitionBytes    int64                   `json:"definitionBytes"`
	Media              []vmBackupManifestMedia `json:"media,omitempty"`
	DiskChecksum       string                  `json:"diskChecksum,omitempty"`
	DiskBytes          int64                   `json:"diskBytes,omitempty"`
	Legacy             bool                    `json:"-"`
}

type vmBackupManifestDisk struct {
	Name     string `json:"name"`
	Target   string `json:"target"`
	Checksum string `json:"checksum"`
	Bytes    int64  `json:"bytes"`
}

type vmBackupManifestMedia struct {
	Name     string `json:"name"`
	Checksum string `json:"checksum"`
	Bytes    int64  `json:"bytes"`
}

func (s *apiServer) reconcileInterruptedBackups() {
	count, err := s.store.FailInterruptedBackupRuns(interruptedBackupReason)
	if err != nil {
		if s.log != nil {
			s.log.Warn("interrupted backup reconciliation failed", "error", err)
		}
		return
	}
	if count > 0 && s.log != nil {
		s.log.Warn("interrupted backups failed closed", "count", count, "reason", interruptedBackupReason)
	}
}

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
		if runs[0].State == "failed" && runs[0].Error != "" {
			status.Warnings = append(status.Warnings, "latest backup failed: "+runs[0].Error)
		}
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
	run := backup.Run{ID: newID("backup-run"), Actor: actor, Trigger: "manual", Generation: s.currentGeneration(), State: "queued", StartedAt: time.Now().UTC()}
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
	if !s.enqueueAutomaticBackup(trigger) {
		return
	}
	go func() {
		for trigger != "" {
			run := backup.Run{ID: newID("backup-run"), Actor: "system", Trigger: trigger, Generation: s.currentGeneration(), State: "queued", StartedAt: time.Now().UTC()}
			if err := s.store.SaveBackupRun(run); err == nil {
				s.executeBackup(run)
			}
			trigger = s.nextAutomaticBackup()
		}
	}()
}

func (s *apiServer) enqueueAutomaticBackup(trigger string) bool {
	s.automaticBackupMu.Lock()
	defer s.automaticBackupMu.Unlock()
	if !s.automaticBackupPending {
		s.automaticBackupPending = true
		return true
	}
	for _, queued := range s.automaticBackupQueue {
		if queued == trigger {
			return false
		}
	}
	s.automaticBackupQueue = append(s.automaticBackupQueue, trigger)
	return false
}

// nextAutomaticBackup returns the next queued trigger, or clears the pending
// state once the queue is empty. Requests arriving after that point start a
// new worker; requests arriving before it are serialized into this worker.
func (s *apiServer) nextAutomaticBackup() string {
	s.automaticBackupMu.Lock()
	defer s.automaticBackupMu.Unlock()
	if len(s.automaticBackupQueue) == 0 {
		s.automaticBackupPending = false
		return ""
	}
	next := s.automaticBackupQueue[0]
	s.automaticBackupQueue = s.automaticBackupQueue[1:]
	return next
}

func (s *apiServer) persistBackupRun(run backup.Run) error {
	if err := s.store.SaveBackupRun(run); err != nil {
		if s.log != nil {
			s.log.Warn("backup state persistence failed", "run", run.ID, "state", run.State, "error", err)
		}
		return err
	}
	return nil
}

func (s *apiServer) executeBackup(run backup.Run) {
	run.State = "running"
	// Do not touch the recovery bundle or remote destinations until the
	// authoritative running state is durable. If the database is unavailable,
	// leaving the queued row for restart reconciliation is safer than doing
	// work that cannot be audited or failed closed.
	if err := s.persistBackupRun(run); err != nil {
		return
	}
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
	s.recoveryExport(recorder, context.Background(), run.Actor)
	if recorder.Code < 200 || recorder.Code >= 300 {
		s.finishBackup(run, "failed", fmt.Errorf("recovery export failed: %s", strings.TrimSpace(recorder.Body.String())))
		return
	}
	var export struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &export); err != nil {
		s.finishBackup(run, "failed", fmt.Errorf("recovery export response was invalid: %w", err))
		return
	}
	if len(export.Warnings) > 0 {
		s.finishBackup(run, "failed", fmt.Errorf("recovery export is incomplete: %s", strings.Join(export.Warnings, "; ")))
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
	if err := s.persistBackupRun(run); err != nil {
		s.finishBackup(run, "failed", fmt.Errorf("persist backup bundle metadata: %w", err))
		return
	}
	vmCtx, vmCancel := context.WithTimeout(context.Background(), 24*time.Hour)
	vmArtifacts, vmErr := s.virtualizationService.BackupManagedDisks(vmCtx, filepath.Dir(bundlePath), run.ID)
	vmCancel()
	if vmErr != nil {
		s.finishBackup(run, "failed", fmt.Errorf("VM backup incomplete: %w", vmErr))
		return
	}
	type vmTransferArtifact struct {
		artifact virtualization.BackupArtifact
		disks    []struct {
			file     virtualization.BackupFileArtifact
			path     string
			checksum string
			bytes    int64
		}
	}
	vmTransfers := make([]vmTransferArtifact, 0, len(vmArtifacts))
	for _, artifact := range vmArtifacts {
		transfer := vmTransferArtifact{artifact: artifact}
		for _, disk := range artifact.Disks {
			sparsePath := disk.Path + ".sparse"
			transferChecksum, transferBytes, packErr := backup.PackSparseFile(disk.Path, sparsePath)
			if packErr != nil {
				s.finishBackup(run, "failed", fmt.Errorf("prepare sparse transfer for VM %q disk %q: %w", artifact.Name, disk.Target, packErr))
				return
			}
			transfer.disks = append(transfer.disks, struct {
				file     virtualization.BackupFileArtifact
				path     string
				checksum string
				bytes    int64
			}{disk, sparsePath, transferChecksum, transferBytes})
		}
		vmTransfers = append(vmTransfers, transfer)
	}
	vmManifest := vmBackupManifest{RunID: run.ID}
	for _, transfer := range vmTransfers {
		artifact := transfer.artifact
		vm := vmBackupManifestVM{Name: artifact.Name, DefinitionChecksum: artifact.Definition.Checksum, DefinitionBytes: artifact.Definition.Bytes}
		for _, disk := range transfer.disks {
			vm.Disks = append(vm.Disks, vmBackupManifestDisk{Name: disk.file.Name, Target: disk.file.Target, Checksum: disk.checksum, Bytes: disk.bytes})
		}
		for _, media := range artifact.Media {
			vm.Media = append(vm.Media, vmBackupManifestMedia{Name: media.Name, Checksum: media.Checksum, Bytes: media.Bytes})
		}
		vmManifest.VMs = append(vmManifest.VMs, vm)
	}
	vmManifestPath := filepath.Join(filepath.Dir(bundlePath), "virtual-machines", run.ID, "manifest.json")
	if err := os.MkdirAll(filepath.Dir(vmManifestPath), 0700); err != nil {
		s.finishBackup(run, "failed", fmt.Errorf("create VM backup manifest directory: %w", err))
		return
	}
	manifestBytes, err := json.Marshal(vmManifest)
	if err != nil {
		s.finishBackup(run, "failed", fmt.Errorf("encode VM backup manifest: %w", err))
		return
	}
	if err := os.WriteFile(vmManifestPath, manifestBytes, 0600); err != nil {
		s.finishBackup(run, "failed", fmt.Errorf("write VM backup manifest: %w", err))
		return
	}
	manifestDigest, manifestSize, err := backup.SHA256File(vmManifestPath)
	if err != nil {
		s.finishBackup(run, "failed", fmt.Errorf("hash VM backup manifest: %w", err))
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
		if err := s.store.SaveBackupCopy(copy); err != nil {
			s.finishBackup(run, "failed", fmt.Errorf("persist backup copy for %s: %w", destination.ID, err))
			return
		}
		verification := backup.Verification{ID: newID("backup-verification"), RunID: run.ID, DestinationID: destination.ID, State: copy.State, Error: copy.Error}
		if copy.Verified {
			now := time.Now().UTC()
			verification.VerifiedAt = &now
		}
		if err := s.store.SaveBackupVerification(verification); err != nil {
			s.finishBackup(run, "failed", fmt.Errorf("persist backup verification for %s: %w", destination.ID, err))
			return
		}
		vmFiles := []struct {
			path     string
			object   string
			checksum string
			bytes    int64
		}{{vmManifestPath, fmt.Sprintf("virtual-machines/%s/manifest.json", run.ID), manifestDigest, manifestSize}}
		for _, transfer := range vmTransfers {
			artifact := transfer.artifact
			vmFiles = append(vmFiles, struct {
				path     string
				object   string
				checksum string
				bytes    int64
			}{artifact.Definition.Path, fmt.Sprintf("virtual-machines/%s/%s/definition.xml", run.ID, artifact.Name), artifact.Definition.Checksum, artifact.Definition.Bytes})
			for _, disk := range transfer.disks {
				vmFiles = append(vmFiles, struct {
					path     string
					object   string
					checksum string
					bytes    int64
				}{disk.path, fmt.Sprintf("virtual-machines/%s/%s/disks/%s.qcow2.sparse", run.ID, artifact.Name, disk.file.Target), disk.checksum, disk.bytes})
			}
			for _, media := range artifact.Media {
				vmFiles = append(vmFiles, struct {
					path     string
					object   string
					checksum string
					bytes    int64
				}{media.Path, fmt.Sprintf("virtual-machines/%s/%s/media/%s", run.ID, artifact.Name, media.Name), media.Checksum, media.Bytes})
			}
		}
		for _, file := range vmFiles {
			vmCopy := backup.Copy{ID: newID("backup-copy"), RunID: run.ID, DestinationID: destination.ID, Object: file.object, Checksum: file.checksum, Bytes: file.bytes, State: "running", CreatedAt: time.Now().UTC()}
			copyCtx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
			copyErr := credentialErr
			if copyErr == nil {
				copyErr = backup.UploadAndVerifyWithRetryContext(copyCtx, full, credentials, file.path, file.object, file.checksum, file.bytes, 3)
			}
			cancel()
			if copyErr != nil {
				vmCopy.State, vmCopy.Error = "failed", copyErr.Error()
				failed++
			} else {
				vmCopy.State, vmCopy.Verified = "verified", true
				now := time.Now().UTC()
				vmCopy.FinishedAt = &now
			}
			if err := s.store.SaveBackupCopy(vmCopy); err != nil {
				s.finishBackup(run, "failed", fmt.Errorf("persist VM backup copy for %s: %w", destination.ID, err))
				return
			}
			vmVerification := backup.Verification{ID: newID("backup-verification"), RunID: run.ID, DestinationID: destination.ID, State: vmCopy.State, Error: vmCopy.Error}
			if vmCopy.Verified {
				now := time.Now().UTC()
				vmVerification.VerifiedAt = &now
			}
			if err := s.store.SaveBackupVerification(vmVerification); err != nil {
				s.finishBackup(run, "failed", fmt.Errorf("persist VM backup verification for %s: %w", destination.ID, err))
				return
			}
		}
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
	s.publishActor(run.Actor, "recovery.backup.verified", "info", nil, map[string]any{"runId": run.ID, "generation": run.Generation, "destinations": len(destinations)})
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
			// A retention lock is deliberately evaluated from the completed
			// copy's creation time, so a later policy reduction cannot erase a
			// recent recovery point. This protects against accidental or
			// ransomware-driven pruning by this daemon; provider-side object
			// lock remains the stronger layer for remote destinations.
			if destination.Retention.ImmutableDays > 0 && copy.CreatedAt.After(time.Now().UTC().AddDate(0, 0, -destination.Retention.ImmutableDays)) {
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
	}
	if err := s.persistBackupRun(run); err != nil {
		return
	}
	if failure != nil {
		s.publishActor(run.Actor, "recovery.backup.failed", "warning", nil, map[string]any{"runId": run.ID, "error": run.Error})
	} else if state == "verified" {
		s.publishActor(run.Actor, "recovery.backup.completed", "info", nil, map[string]any{"runId": run.ID})
	}
}

func (s *apiServer) backupDestinationRestoreCheck(w http.ResponseWriter, r *http.Request, destinationID string) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	key := []byte(s.recoveryKeyString())
	if len(key) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is not configured"})
		return
	}
	destination, credentials, err := s.store.BackupDestination(destinationID, key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "backup destination not found or credentials unavailable"})
		return
	}
	copies, err := s.store.BackupCopiesForDestination(destinationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var selected *backup.Copy
	for index := range copies {
		if copies[index].State == "verified" && copies[index].Verified && strings.HasSuffix(copies[index].Object, ".mrb") {
			selected = &copies[index]
			break
		}
	}
	if selected == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no verified backup copy exists at this destination"})
		return
	}
	verification := backup.Verification{ID: newID("backup-verification"), RunID: selected.RunID, DestinationID: destinationID, State: "failed"}
	var checkErr error
	root, tempErr := os.MkdirTemp("", "lumonas-restore-check-")
	if tempErr != nil {
		checkErr = tempErr
	} else {
		defer os.RemoveAll(root)
		archivePath := filepath.Join(root, "recovery.bundle")
		if err := backup.DownloadWithTimeout(destination, credentials, selected.Object, archivePath); err != nil {
			checkErr = fmt.Errorf("download copy: %w", err)
		} else if digest, size, err := backup.SHA256File(archivePath); err != nil {
			checkErr = err
		} else if digest != selected.Checksum || size != selected.Bytes {
			checkErr = errors.New("downloaded copy checksum or size does not match the recorded copy")
		} else if payload, err := os.ReadFile(archivePath); err != nil {
			checkErr = err
		} else if _, err := recovery.Verify(payload, key); err != nil {
			checkErr = fmt.Errorf("verify recovery bundle: %w", err)
		} else if result, err := recovery.Apply(payload, key, recovery.ApplyOptions{Root: filepath.Join(root, "restore")}); err != nil {
			checkErr = fmt.Errorf("temporary restore: %w", err)
		} else if err := s.verifyRemoteVMBackup(destination, credentials, copies, selected.RunID, root); err != nil {
			checkErr = fmt.Errorf("verify VM recovery artifacts: %w", err)
		} else {
			verification.State = "verified"
			now := time.Now().UTC()
			verification.VerifiedAt = &now
			_ = s.store.SaveAudit(store.AuditEntry{Actor: actor, Action: "backup.destination.restore_check", Outcome: "verified", ResourceType: "backup-destination", ResourceID: destinationID, Metadata: map[string]any{"runId": selected.RunID, "appliedFiles": len(result.AppliedFiles), "appdataRestored": len(result.AppdataRestored), "databaseRestored": result.DatabaseRestored}})
			if err := s.store.SaveBackupVerification(verification); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "restore check passed but verification record could not be saved"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"state": "verified", "destinationId": destinationID, "runId": selected.RunID, "checksum": digest, "bytes": size, "databaseRestored": result.DatabaseRestored, "appdataRestored": result.AppdataRestored, "appliedFiles": len(result.AppliedFiles), "serviceStartup": "not_attempted"})
			return
		}
	}
	verification.Error = checkErr.Error()
	_ = s.store.SaveBackupVerification(verification)
	_ = s.store.SaveAudit(store.AuditEntry{Actor: actor, Action: "backup.destination.restore_check", Outcome: "failed", ResourceType: "backup-destination", ResourceID: destinationID, Metadata: map[string]any{"runId": selected.RunID, "error": checkErr.Error()}})
	writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": checkErr.Error()})
}

func (s *apiServer) verifyRemoteVMBackup(destination backup.Destination, credentials backup.Credentials, copies []backup.Copy, runID, root string) error {
	manifestObject := fmt.Sprintf("virtual-machines/%s/manifest.json", runID)
	var manifestCopy *backup.Copy
	for index := range copies {
		if copies[index].RunID == runID && copies[index].Object == manifestObject {
			manifestCopy = &copies[index]
			break
		}
	}
	if manifestCopy == nil {
		return nil // Legacy generations predate VM backup manifests.
	}
	if manifestCopy.State != "verified" || !manifestCopy.Verified {
		return errors.New("VM backup manifest copy is not verified")
	}
	manifestPath := filepath.Join(root, "vm-manifest.json")
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	if err := backup.Download(ctx, destination, credentials, manifestCopy.Object, manifestPath); err != nil {
		return fmt.Errorf("download VM backup manifest: %w", err)
	}
	if digest, size, err := backup.SHA256File(manifestPath); err != nil {
		return err
	} else if digest != manifestCopy.Checksum || size != manifestCopy.Bytes {
		return errors.New("VM backup manifest checksum does not match its record")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest vmBackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.RunID != runID || len(manifest.VMs) > 128 {
		return errors.New("VM backup manifest is invalid")
	}
	if err := normalizeLegacyVMManifest(&manifest); err != nil {
		return err
	}
	seen := make(map[string]bool, len(manifest.VMs))
	for _, vm := range manifest.VMs {
		if !validVMBackupName(vm.Name) || seen[vm.Name] || len(vm.Disks) < 1 || len(vm.Disks) > 32 || vm.DefinitionBytes < 0 {
			return errors.New("VM backup manifest contains an invalid entry")
		}
		seen[vm.Name] = true
		vmRoot := filepath.Join(root, "vm-"+vm.Name)
		definitionObject := fmt.Sprintf("virtual-machines/%s/%s/definition.xml", runID, vm.Name)
		if vm.Legacy {
			definitionObject = fmt.Sprintf("virtual-machines/%s/%s.xml", runID, vm.Name)
		}
		definitionPath := filepath.Join(vmRoot, "definition.xml")
		if err := os.MkdirAll(vmRoot, 0700); err != nil {
			return err
		}
		if err := s.downloadVerifiedVMArtifact(ctx, destination, credentials, copies, runID, definitionObject, definitionPath, vm.DefinitionChecksum, vm.DefinitionBytes); err != nil {
			return fmt.Errorf("VM %q definition: %w", vm.Name, err)
		}
		definition, err := os.ReadFile(definitionPath)
		if err != nil {
			return err
		}
		diskTargets := make(map[string]bool)
		diskInventory := make(map[string]string, len(vm.Disks))
		for _, disk := range vm.Disks {
			if !validVMDiskTarget(disk.Target) || filepath.Base(disk.Name) != disk.Name || filepath.Ext(disk.Name) != ".qcow2" || disk.Bytes < 0 || diskTargets[disk.Target] {
				return fmt.Errorf("VM %q manifest has an invalid disk entry", vm.Name)
			}
			diskTargets[disk.Target] = true
			diskInventory[disk.Target] = disk.Name
		}
		mediaInventory := make([]string, 0, len(vm.Media))
		for _, media := range vm.Media {
			if filepath.Base(media.Name) != media.Name || !strings.EqualFold(filepath.Ext(media.Name), ".iso") || media.Bytes < 0 {
				return fmt.Errorf("VM %q manifest has an invalid installer media entry", vm.Name)
			}
			mediaInventory = append(mediaInventory, media.Name)
		}
		if err := s.virtualizationService.ValidateBackupDefinitionInventory(vm.Name, definition, diskInventory, mediaInventory); err != nil {
			return fmt.Errorf("VM %q definition inventory is unsafe: %w", vm.Name, err)
		}
		for _, disk := range vm.Disks {
			object := fmt.Sprintf("virtual-machines/%s/%s/disks/%s.qcow2.sparse", runID, vm.Name, disk.Target)
			if vm.Legacy {
				sparseObject := fmt.Sprintf("virtual-machines/%s/%s.qcow2.sparse", runID, vm.Name)
				rawObject := fmt.Sprintf("virtual-machines/%s/%s.qcow2", runID, vm.Name)
				if hasBackupCopy(copies, runID, sparseObject) {
					object = sparseObject
				} else {
					object = rawObject
				}
			}
			diskRoot := filepath.Join(vmRoot, "disks")
			if err := os.MkdirAll(diskRoot, 0700); err != nil {
				return err
			}
			packedPath := filepath.Join(diskRoot, disk.Target+".qcow2.sparse")
			if vm.Legacy && strings.HasSuffix(object, ".qcow2") && !strings.HasSuffix(object, ".qcow2.sparse") {
				packedPath = filepath.Join(diskRoot, disk.Target+".qcow2")
			}
			if err := s.downloadVerifiedVMArtifact(ctx, destination, credentials, copies, runID, object, packedPath, disk.Checksum, disk.Bytes); err != nil {
				return fmt.Errorf("VM %q disk %q: %w", vm.Name, disk.Target, err)
			}
			unpackedPath := filepath.Join(vmRoot, disk.Name)
			if vm.Legacy && strings.HasSuffix(object, ".qcow2") && !strings.HasSuffix(object, ".qcow2.sparse") {
				if err := os.Rename(packedPath, unpackedPath); err != nil {
					return err
				}
			} else if err := backup.UnpackSparseFile(packedPath, unpackedPath); err != nil {
				return fmt.Errorf("VM %q disk %q package is invalid: %w", vm.Name, disk.Target, err)
			}
			if err := s.virtualizationService.CheckBackupDisk(ctx, unpackedPath); err != nil {
				return fmt.Errorf("VM %q disk %q: %w", vm.Name, disk.Target, err)
			}
		}
		mediaNames := make(map[string]bool)
		for _, media := range vm.Media {
			if filepath.Base(media.Name) != media.Name || !strings.EqualFold(filepath.Ext(media.Name), ".iso") || media.Bytes < 0 || mediaNames[media.Name] {
				return fmt.Errorf("VM %q manifest has an invalid installer media entry", vm.Name)
			}
			mediaNames[media.Name] = true
			object := fmt.Sprintf("virtual-machines/%s/%s/media/%s", runID, vm.Name, media.Name)
			mediaRoot := filepath.Join(vmRoot, "media")
			if err := os.MkdirAll(mediaRoot, 0700); err != nil {
				return err
			}
			mediaPath := filepath.Join(mediaRoot, media.Name)
			if err := s.downloadVerifiedVMArtifact(ctx, destination, credentials, copies, runID, object, mediaPath, media.Checksum, media.Bytes); err != nil {
				return fmt.Errorf("VM %q installer media %q: %w", vm.Name, media.Name, err)
			}
			if err := s.virtualizationService.ValidateBackupMedia(media.Name, mediaPath); err != nil {
				return fmt.Errorf("VM %q installer media %q: %w", vm.Name, media.Name, err)
			}
		}
	}
	return nil
}

func normalizeLegacyVMManifest(manifest *vmBackupManifest) error {
	for index := range manifest.VMs {
		vm := &manifest.VMs[index]
		if len(vm.Disks) == 0 && vm.DiskChecksum != "" {
			if vm.DiskBytes < 0 {
				return errors.New("legacy VM backup manifest has an invalid disk size")
			}
			vm.Legacy = true
			vm.Disks = []vmBackupManifestDisk{{Name: vm.Name + ".qcow2", Target: "vda", Checksum: vm.DiskChecksum, Bytes: vm.DiskBytes}}
		}
	}
	return nil
}

func hasBackupCopy(copies []backup.Copy, runID, object string) bool {
	for _, copyRecord := range copies {
		if copyRecord.RunID == runID && copyRecord.Object == object && copyRecord.State == "verified" && copyRecord.Verified {
			return true
		}
	}
	return false
}

func (s *apiServer) downloadVerifiedVMArtifact(ctx context.Context, destination backup.Destination, credentials backup.Credentials, copies []backup.Copy, runID, object, target, checksum string, bytes int64) error {
	var copyRecord *backup.Copy
	for index := range copies {
		if copies[index].RunID == runID && copies[index].Object == object {
			copyRecord = &copies[index]
			break
		}
	}
	if copyRecord == nil || copyRecord.State != "verified" || !copyRecord.Verified || copyRecord.Checksum != checksum || copyRecord.Bytes != bytes {
		return errors.New("remote recovery copy is missing or unverified")
	}
	if err := backup.Download(ctx, destination, credentials, object, target); err != nil {
		return err
	}
	digest, size, err := backup.SHA256File(target)
	if err != nil {
		return err
	}
	if digest != checksum || size != bytes {
		return errors.New("remote recovery copy checksum or size mismatch")
	}
	return nil
}

func validVMDiskTarget(target string) bool {
	if !validVMBackupName(target) {
		return false
	}
	return true
}

func (s *apiServer) restoreBackupVirtualMachine(w http.ResponseWriter, r *http.Request, destinationID, runID, name string) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	if !validVMBackupName(name) || !validVMBackupName(runID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "VM name is invalid"})
		return
	}
	var input struct {
		ConfirmName string `json:"confirmName"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&input); err != nil || input.ConfirmName != name {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type the VM name exactly to confirm restoration"})
		return
	}
	key := []byte(s.recoveryKeyString())
	if len(key) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is not configured"})
		return
	}
	destination, credentials, err := s.store.BackupDestination(destinationID, key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "backup destination not found or credentials unavailable"})
		return
	}
	copies, err := s.store.BackupCopiesForDestination(destinationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var run backup.Run
	for _, candidate := range copies {
		if candidate.RunID == runID && candidate.State == "verified" && candidate.Verified && candidate.Object == fmt.Sprintf("virtual-machines/%s/manifest.json", runID) {
			run.ID = runID
			break
		}
	}
	if run.ID == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "verified VM backup generation not found"})
		return
	}
	root, err := os.MkdirTemp("", "lumonas-vm-restore-")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create temporary VM restore workspace"})
		return
	}
	defer os.RemoveAll(root)
	if err := s.verifyRemoteVMBackup(destination, credentials, copies, runID, root); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "VM backup verification failed: " + err.Error()})
		return
	}
	manifestData, err := os.ReadFile(filepath.Join(root, "vm-manifest.json"))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "VM backup manifest is unavailable"})
		return
	}
	var manifest vmBackupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "VM backup manifest is invalid"})
		return
	}
	if err := normalizeLegacyVMManifest(&manifest); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	var vmEntry *vmBackupManifestVM
	for index := range manifest.VMs {
		if manifest.VMs[index].Name == name {
			vmEntry = &manifest.VMs[index]
			break
		}
	}
	if vmEntry == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "VM is not present in this backup generation"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 24*time.Hour)
	defer cancel()
	vmRoot := filepath.Join(root, "vm-"+name)
	diskSources := make(map[string]string, len(vmEntry.Disks))
	for _, disk := range vmEntry.Disks {
		diskSources[disk.Name] = filepath.Join(vmRoot, disk.Name)
	}
	mediaSources := make(map[string]string, len(vmEntry.Media))
	for _, media := range vmEntry.Media {
		mediaSources[media.Name] = filepath.Join(vmRoot, "media", media.Name)
	}
	restored, err := s.virtualizationService.RestoreBackupGuest(ctx, name, filepath.Join(vmRoot, "definition.xml"), diskSources, mediaSources)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "backup.vm.restore", name, map[string]any{"destinationId": destinationID, "runId": runID, "state": restored.State})
	s.publishActor(actor, "backup.vm.restored", "warning", nil, map[string]any{"name": name, "destinationId": destinationID, "runId": runID, "state": restored.State})
	writeJSON(w, http.StatusCreated, restored)
}

func validVMBackupName(name string) bool {
	if len(name) < 1 || len(name) > 63 || !isASCIIAlphaNumeric(name[0]) {
		return false
	}
	for index := 1; index < len(name); index++ {
		if !isASCIIAlphaNumeric(name[index]) && name[index] != '_' && name[index] != '.' && name[index] != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
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
		if err != nil {
			continue
		}
		s.checkUSBBackupAttachments(schedule)
		if !schedule.Enabled {
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

func (s *apiServer) checkUSBBackupAttachments(schedule backup.Schedule) {
	destinations, err := s.store.ListBackupDestinations()
	if err != nil {
		return
	}
	if s.usbMountState == nil {
		s.usbMountState = make(map[string]bool)
	}
	for _, destination := range destinations {
		removable := destination.Type == backup.DestinationLocal && backup.IsRemovableBackupTarget(destination.Target)
		mounted := removable && destination.Enabled && backup.RemovableBackupTargetMounted(destination.Target)
		if mounted && !s.usbMountState[destination.ID] && schedule.OnUSBAttach {
			s.requestAutomaticBackup("usb.attach")
		}
		s.usbMountState[destination.ID] = mounted
	}
}

func (s *apiServer) dockerAppdataCoverage() (bool, []string) {
	var plan *recovery.RestorePlan
	bundlePath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	if key := s.recoveryKeyString(); key != "" {
		if bundle, readErr := os.ReadFile(bundlePath); readErr == nil {
			if value, planErr := recovery.Plan(bundle, []byte(key)); planErr == nil {
				plan = &value
			}
		}
	}
	return s.dockerAppdataCoverageForPlan(plan)
}

func (s *apiServer) dockerAppdataCoverageForPlan(plan *recovery.RestorePlan) (bool, []string) {
	stacks, err := s.decoratedDockerStacks(context.Background())
	if err != nil {
		return false, []string{"Docker appdata coverage could not be determined"}
	}
	if len(stacks) == 0 {
		return true, nil
	}
	covered := make(map[string]map[string]bool)
	if plan != nil {
		for _, record := range plan.Appdata {
			if covered[record.Stack] == nil {
				covered[record.Stack] = make(map[string]bool)
			}
			covered[record.Stack][record.ContainerPath] = true
		}
	}
	warnings := make([]string, 0, len(stacks)+1)
	fullyCovered := true
	for _, stack := range stacks {
		if stack.Recovery == nil || len(stack.Recovery.AppdataPaths) == 0 || stack.Recovery.Strategy == dockerruntime.StrategyNone {
			warnings = append(warnings, stack.Name+" has no configured appdata recovery contract")
			fullyCovered = false
			continue
		}
		for _, path := range stack.Recovery.AppdataPaths {
			if !covered[stack.Name][path] {
				warnings = append(warnings, stack.Name+" appdata requires a verified content backup for "+path)
				fullyCovered = false
			}
		}
	}
	if !fullyCovered && len(warnings) == 0 {
		warnings = append(warnings, "Docker appdata is not covered by the latest verified recovery bundle")
	}
	return fullyCovered, warnings
}
