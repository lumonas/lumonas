package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/recovery"
)

// These handlers keep the first UI API shape compatible while the richer
// destination/run API remains available under /backups/*.
func (s *apiServer) backupReadiness(w http.ResponseWriter, r *http.Request) {
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
	keyReady := s.recoveryKeyString() != ""
	latestReady := len(runs) > 0 && runs[0].State == "verified"
	dockerCovered, dockerWarnings := s.dockerAppdataCoverage()
	layers := []map[string]any{
		{"id": "recovery-key", "label": "Recovery key", "status": readinessStatus(keyReady), "detail": readinessDetail(keyReady, "Configured", "Not configured")},
		{"id": "destinations", "label": "Backup destinations", "status": readinessStatus(len(destinations) > 0), "detail": readinessDetail(len(destinations) > 0, "Configured", "No destination configured")},
		{"id": "latest-bundle", "label": "Latest verified bundle", "status": readinessStatus(latestReady), "detail": readinessDetail(latestReady, "Verified", "No verified bundle")},
		{"id": "docker-appdata", "label": "Docker appdata coverage", "status": readinessStatus(dockerCovered), "detail": readinessDetail(dockerCovered, "Covered", "Not fully recoverable")},
	}
	score := 0
	for _, layer := range layers {
		if layer["status"] == "current" {
			score += 100 / len(layers)
		}
	}
	result := map[string]any{"score": score, "layers": layers}
	if len(dockerWarnings) > 0 {
		result["warnings"] = dockerWarnings
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) backupJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	destinations, err := s.store.ListBackupDestinations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	runs, err := s.store.BackupRuns(50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	values := make([]map[string]any, 0, len(destinations))
	for _, destination := range destinations {
		var lastRun map[string]any
		if len(runs) > 0 {
			run := runs[0]
			status := "offline"
			switch run.State {
			case "verified":
				status = "healthy"
			case "failed":
				status = "critical"
			case "queued", "running":
				status = "attention"
			}
			at := run.StartedAt
			if run.FinishedAt != nil {
				at = *run.FinishedAt
			}
			lastRun = map[string]any{"status": status, "at": at, "detail": run.Error}
		}
		values = append(values, map[string]any{
			"id": destination.ID, "name": destination.Name, "source": "configuration",
			"destinationId": destination.ID, "schedule": "On demand / automatic",
			"strategy": "verified encrypted bundle", "jobType": "recovery.bundle",
			"lastRun": lastRun, "enabled": destination.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) backupDestinationSummaries(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	destinations, err := s.store.ListBackupDestinations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	verifications, _ := s.store.BackupVerifications(500)
	values := make([]map[string]any, 0, len(destinations))
	for _, destination := range destinations {
		kind := "nas"
		switch destination.Type {
		case backup.DestinationS3:
			kind = "s3"
		case backup.DestinationSFTP:
			kind = "sftp"
		case backup.DestinationLocal:
			if strings.HasPrefix(destination.Target, "/media/") || strings.HasPrefix(destination.Target, "/mnt/") {
				kind = "usb"
			}
		}
		status := "offline"
		if destination.Enabled {
			status = "attention"
		}
		var lastVerified *time.Time
		for _, verification := range verifications {
			if verification.DestinationID != destination.ID {
				continue
			}
			if verification.State == "verified" && verification.VerifiedAt != nil {
				status = "healthy"
				if lastVerified == nil || verification.VerifiedAt.After(*lastVerified) {
					lastVerified = verification.VerifiedAt
				}
			} else if status != "healthy" {
				status = "critical"
			}
		}
		value := map[string]any{
			"id": destination.ID, "label": destination.Name, "type": kind,
			"target": destination.Target, "encrypted": true, "status": status,
			"detail": "Encrypted recovery bundle",
		}
		if lastVerified != nil {
			value["lastVerifiedAt"] = lastVerified
		}
		values = append(values, value)
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) runBackupJob(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	destinations, err := s.store.ListBackupDestinations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	valid := false
	for _, destination := range destinations {
		if destination.ID == id {
			valid = true
			break
		}
	}
	if !valid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "backup job not found"})
		return
	}
	run := backup.Run{ID: newID("backup-run"), Trigger: "manual", Generation: s.currentGeneration(), State: "queued", StartedAt: time.Now().UTC()}
	if err := s.store.SaveBackupRun(run); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "backup.run.request", run.ID, map[string]any{"destinationId": id})
	go s.executeBackup(run)
	writeJSON(w, http.StatusAccepted, map[string]any{"id": run.ID, "type": "backup", "title": "Configuration backup", "resourceId": id, "state": "queued", "progress": 0, "createdAt": run.StartedAt})
}

func (s *apiServer) backupGenerations(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	values, err := s.store.ConfigGenerations(50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		status := "committed"
		if value.State != "committed" {
			status = "failed"
		}
		result = append(result, map[string]any{"id": value.ID, "createdAt": value.CreatedAt, "actor": "system", "status": status, "summary": value.PlanHash, "config": value.PlanHash})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) backupRestorePlan(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	result := map[string]any{
		"generationId":  s.currentGeneration(),
		"interfaces":    []any{},
		"apps":          []any{},
		"dataDisksNote": "No verified recovery bundle is available yet; data disks will remain read-only until their stable identities are confirmed.",
	}
	key := s.recoveryKeyString()
	bundle, err := os.ReadFile(filepath.Join(envOr("MYNAS_RECOVERY_DIR", "/var/lib/mynas/recovery"), "latest.mrb"))
	if key == "" || err != nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	plan, err := recovery.Plan(bundle, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	apps := make([]map[string]any, 0)
	for _, name := range plan.Files {
		if !strings.HasPrefix(name, "docker/stacks/") || !strings.HasSuffix(name, "/compose.yaml") {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) == 4 {
			apps = append(apps, map[string]any{"name": parts[2], "appdataAvailable": true})
		}
	}
	result["generationId"] = plan.Manifest.Generation
	result["apps"] = apps
	result["dataDisksNote"] = fmt.Sprintf("Recovery bundle generation %d includes %d recorded disk identity(ies). Data disks will be imported read-only before any write operation.", plan.Manifest.Generation, len(plan.Manifest.DiskIDs))
	writeJSON(w, http.StatusOK, result)
}

func readinessStatus(ready bool) string {
	if ready {
		return "current"
	}
	return "missing"
}

func readinessDetail(ready bool, current, missing string) string {
	if ready {
		return current
	}
	return missing
}
