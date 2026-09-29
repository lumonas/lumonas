package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/network"
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
	var latestPlan *recovery.RestorePlan
	var latestBackupAt *time.Time
	latestReady := false
	if keyReady && len(runs) > 0 && runs[0].State == "verified" && runs[0].BundlePath != "" {
		bundle, readErr := os.ReadFile(runs[0].BundlePath)
		if readErr == nil {
			digest, _, digestErr := backup.SHA256File(runs[0].BundlePath)
			plan, planErr := recovery.Plan(bundle, []byte(s.recoveryKeyString()))
			if digestErr == nil && planErr == nil && plan.Verified && (runs[0].Checksum == "" || digest == runs[0].Checksum) {
				latestPlan = &plan
				latestReady = true
				latestBackupAt = runs[0].FinishedAt
				if latestBackupAt == nil {
					latestBackupAt = &runs[0].StartedAt
				}
			}
		}
	}
	shares, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "managed shares could not be loaded"})
		return
	}
	shareCoverage, coveredShares := make([]map[string]any, 0, len(shares)), 0
	archivedShares := map[string]bool{}
	if latestPlan != nil {
		for _, item := range latestPlan.Shares {
			archivedShares[item.ID] = true
		}
	}
	now := time.Now().UTC()
	staleAfter := 36 * time.Hour
	if schedule, scheduleErr := s.store.BackupSchedule(); scheduleErr == nil && schedule.IntervalSeconds > 0 {
		candidate := time.Duration(schedule.IntervalSeconds) * time.Second * 2
		if candidate > staleAfter {
			staleAfter = candidate
		}
	}
	for _, share := range shares {
		covered := latestReady && archivedShares[share.ID]
		status, detail := readinessStatus(covered), "Not present in the latest verified bundle"
		if covered {
			coveredShares++
			detail = "Included in the latest verified bundle"
			if latestBackupAt != nil && now.Sub(*latestBackupAt) > staleAfter {
				status = "stale"
				detail = "Included in a verified bundle, but the latest copy is overdue"
			}
		}
		shareCoverage = append(shareCoverage, map[string]any{"id": share.ID, "name": share.Name, "kind": "share", "status": status, "detail": detail, "lastSuccessfulAt": latestBackupAt})
	}
	dockerCovered, dockerWarnings := s.dockerAppdataCoverageForPlan(latestPlan)
	dockerDetail := "All configured app-data paths are present in the latest bundle"
	if !dockerCovered {
		dockerDetail = "Docker app-data coverage is incomplete"
		if len(dockerWarnings) > 0 {
			dockerDetail = strings.Join(dockerWarnings, "; ")
		}
	}
	coverage := append(shareCoverage, map[string]any{"id": "docker-appdata", "name": "Docker app data", "kind": "docker-appdata", "status": readinessStatus(dockerCovered), "detail": dockerDetail, "lastSuccessfulAt": latestBackupAt})
	if latestReady && latestBackupAt != nil && now.Sub(*latestBackupAt) > staleAfter {
		for _, item := range coverage {
			if item["status"] == "current" {
				item["status"] = "stale"
			}
		}
	}
	restoreDrills, drillErr := s.store.RestoreDrills(1)
	drillCurrent := drillErr == nil && len(restoreDrills) > 0 && restoreDrills[0].State == "successful" && restoreDrills[0].Verified && restoreDrills[0].DatabaseRestored && restoreDrills[0].ServicesHealthy
	drillDetail := "No successful restore drill recorded"
	var lastDrillAt *time.Time
	if drillErr == nil && len(restoreDrills) > 0 {
		lastDrillAt = restoreDrills[0].FinishedAt
		if restoreDrills[0].State == "failed" {
			drillDetail = "Latest restore drill failed: " + restoreDrills[0].Error
		} else if drillCurrent {
			drillDetail = "Verified restore drill passed"
			if lastDrillAt != nil && now.Sub(*lastDrillAt) > 30*24*time.Hour {
				drillCurrent = false
				drillDetail = "Last restore drill passed, but is more than 30 days old"
			}
		} else {
			drillDetail = "Latest restore drill did not verify every recovery layer"
		}
	}
	latestBundleStatus := readinessStatus(latestReady)
	shareCoverageCurrent := len(shares) == coveredShares
	if latestReady && latestBackupAt != nil && now.Sub(*latestBackupAt) > staleAfter {
		latestBundleStatus = "stale"
		shareCoverageCurrent = false
	}
	layers := []map[string]any{
		{"id": "recovery-key", "label": "Recovery key", "status": readinessStatus(keyReady), "detail": readinessDetail(keyReady, "Configured", "Not configured")},
		{"id": "destinations", "label": "Backup destinations", "status": readinessStatus(len(destinations) > 0), "detail": readinessDetail(len(destinations) > 0, "Configured", "No destination configured")},
		{"id": "latest-bundle", "label": "Latest verified bundle", "status": latestBundleStatus, "detail": readinessDetail(latestReady, readinessDetail(latestBundleStatus == "current", "Verified and checksum-matched", "Verified bundle is overdue"), "No verified bundle matches its recorded checksum")},
		{"id": "share-coverage", "label": "Managed share coverage", "status": readinessStatus(shareCoverageCurrent), "detail": fmt.Sprintf("%d of %d managed shares are included in a fresh verified bundle", coveredShares, len(shares))},
		{"id": "docker-appdata", "label": "Docker appdata coverage", "status": readinessStatus(dockerCovered), "detail": readinessDetail(dockerCovered, "Covered", "Not fully recoverable")},
		{"id": "restore-drill", "label": "Restore drill", "status": readinessStatus(drillCurrent), "detail": drillDetail, "lastSuccessfulAt": lastDrillAt},
	}
	scoreTotal := 0.0
	for _, layer := range layers {
		switch layer["status"] {
		case "current":
			scoreTotal += 1
		case "stale":
			scoreTotal += 0.6
		}
	}
	score := int(math.Round(scoreTotal * 100 / float64(len(layers))))
	result := map[string]any{"score": score, "layers": layers, "coverage": coverage}
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
		case backup.DestinationRclone:
			kind = "cloud"
		case backup.DestinationLocal:
			if backup.IsRemovableBackupTarget(destination.Target) {
				kind = "usb"
			}
		}
		status := "offline"
		mounted := true
		if destination.Enabled {
			status = "attention"
		}
		if kind == "usb" {
			mounted = backup.RemovableBackupTargetMounted(destination.Target)
			if destination.Enabled && !mounted {
				status = "offline"
			}
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
		if kind == "usb" && destination.Enabled && !mounted {
			status = "offline"
		}
		value := map[string]any{
			"id": destination.ID, "label": destination.Name, "type": kind,
			"target": destination.Target, "encrypted": true, "status": status, "enabled": destination.Enabled,
			"detail": readinessDetail(mounted, "Encrypted recovery bundle", "USB destination is not mounted"), "immutableDays": destination.Retention.ImmutableDays, "providerObjectLock": destination.Retention.ProviderObjectLock,
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
	run := backup.Run{ID: newID("backup-run"), Actor: actor, Trigger: "manual", Generation: s.currentGeneration(), State: "queued", StartedAt: time.Now().UTC()}
	if err := s.store.SaveBackupRun(run); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "backup.run.request", run.ID, map[string]any{"operationId": run.ID, "destinationId": id})
	go s.executeBackup(run)
	writeJSON(w, http.StatusAccepted, map[string]any{"id": run.ID, "actor": run.Actor, "type": "backup", "title": "Configuration backup", "resourceId": id, "state": "queued", "progress": 0, "createdAt": run.StartedAt})
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
	bundle, err := os.ReadFile(filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb"))
	if key == "" || err != nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	plan, err := recovery.Plan(bundle, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	_, verifiedFiles, err := recovery.ReadVerified(bundle, []byte(key))
	if err == nil {
		var savedConnections []network.Connection
		if json.Unmarshal(verifiedFiles["config/network-connections.json"], &savedConnections) == nil {
			available := make([]string, 0)
			if live, liveErr := network.Interfaces(); liveErr == nil {
				for _, iface := range live {
					if !iface.Loopback {
						available = append(available, iface.Name)
					}
				}
			}
			interfaces := make([]map[string]any, 0, len(savedConnections))
			for _, connection := range savedConnections {
				if !connection.Enabled || connection.Interface == "" {
					continue
				}
				detail := connection.Name
				if len(connection.IPv4.Addresses) > 0 {
					detail += " · " + connection.IPv4.Addresses[0]
				}
				interfaces = append(interfaces, map[string]any{"old": connection.Interface, "detail": detail, "options": available})
			}
			result["interfaces"] = interfaces
		}
	}
	apps := make([]map[string]any, 0)
	appdata := make(map[string]bool)
	for _, record := range plan.Appdata {
		appdata[record.Stack] = true
	}
	for _, name := range plan.Files {
		if !strings.HasPrefix(name, "docker/stacks/") || !strings.HasSuffix(name, "/compose.yaml") {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) == 4 {
			apps = append(apps, map[string]any{"name": parts[2], "appdataAvailable": appdata[parts[2]]})
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
