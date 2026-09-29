package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/store"
)

var restoreDrillMu sync.Mutex

func (s *apiServer) restoreDrills(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.RestoreDrills(50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) workloadRecoveryObjectives(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.WorkloadRecoveryObjectives()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) updateWorkloadRecoveryObjective(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var value store.WorkloadRecoveryObjective
	if err := jsonDecode(r, &value); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.store.SetWorkloadRecoveryObjective(value); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	_ = s.store.SaveAudit(store.AuditEntry{Actor: actor, Action: "recovery.workload_objective.update", Outcome: "recorded", ResourceType: "docker-stack", ResourceID: value.WorkloadID, Metadata: map[string]any{"rpoHours": value.RPOHours, "rtoMinutes": value.RTOMinutes}})
	values, _ := s.store.WorkloadRecoveryObjectives()
	for _, current := range values {
		if current.WorkloadID == value.WorkloadID {
			writeJSON(w, http.StatusOK, current)
			return
		}
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) restoreDrillSchedule(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	value, err := s.store.RestoreDrillSchedule()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) updateRestoreDrillSchedule(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	value, err := s.store.RestoreDrillSchedule()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var input struct {
		Enabled         *bool  `json:"enabled"`
		IntervalSeconds *int64 `json:"intervalSeconds"`
		RPOHours        *int   `json:"rpoHours"`
		RTOMinutes      *int   `json:"rtoMinutes"`
	}
	if err := jsonDecode(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.Enabled != nil {
		value.Enabled = *input.Enabled
	}
	if input.IntervalSeconds != nil {
		value.IntervalSeconds = *input.IntervalSeconds
		value.NextDueAt = nil
	}
	if input.RPOHours != nil {
		value.RPOHours = *input.RPOHours
	}
	if input.RTOMinutes != nil {
		value.RTOMinutes = *input.RTOMinutes
	}
	if value.IntervalSeconds < 3600 || value.IntervalSeconds > 365*24*60*60 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "intervalSeconds must be between one hour and one year"})
		return
	}
	if value.RPOHours < 1 || value.RPOHours > 8760 || value.RTOMinutes < 1 || value.RTOMinutes > 10080 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "rpoHours must be 1-8760 and rtoMinutes must be 1-10080"})
		return
	}
	if value.Enabled && value.NextDueAt == nil {
		next := time.Now().UTC().Add(time.Duration(value.IntervalSeconds) * time.Second)
		value.NextDueAt = &next
	}
	if _, err := s.store.SaveRestoreDrillSchedule(value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "recovery.restore_drill_schedule.update", "restore-drill-schedule", map[string]any{"enabled": value.Enabled, "intervalSeconds": value.IntervalSeconds})
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) runRestoreDrillNow(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	value, started := s.startRestoreDrill("manual", actor)
	if !started {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a restore drill is already running"})
		return
	}
	s.recordRequestAudit(r, actor, "recovery.restore_drill.request", value.ID, nil)
	writeJSON(w, http.StatusAccepted, value)
}

func (s *apiServer) startRestoreDrill(trigger, _ string) (model.RestoreDrill, bool) {
	restoreDrillMu.Lock()
	defer restoreDrillMu.Unlock()
	existing, _ := s.store.RestoreDrills(10)
	for _, drill := range existing {
		if drill.State == "running" {
			return drill, false
		}
	}
	now := time.Now().UTC()
	value := model.RestoreDrill{ID: newID("restore-drill"), Trigger: trigger, State: "running", StartedAt: now}
	if err := s.store.SaveRestoreDrill(value); err != nil {
		return value, false
	}
	go s.executeRestoreDrill(value)
	return value, true
}

func (s *apiServer) executeRestoreDrill(value model.RestoreDrill) {
	bundlePath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	value.BundlePath = bundlePath
	bundle, err := os.ReadFile(bundlePath)
	if err == nil {
		key := s.recoveryKeyString()
		if key == "" {
			err = errors.New("recovery key is not configured")
		} else {
			plan, planErr := recovery.Plan(bundle, []byte(key))
			if planErr != nil {
				err = planErr
			} else {
				value.Generation = plan.Manifest.Generation
				value.Verified, value.DatabaseValid, value.ComposeValid = plan.Verified, plan.DatabaseValid, plan.ComposeValid
				value.Warnings = append(value.Warnings, plan.Warnings...)
				if !plan.DatabaseValid || !plan.DesiredStateValid || !plan.ComposeValid {
					err = errors.New("restore payload validation failed")
				}
				if err == nil {
					root, tempErr := os.MkdirTemp("", "lumonas-restore-drill-")
					if tempErr != nil {
						err = tempErr
					} else {
						defer os.RemoveAll(root)
						result, applyErr := recovery.Apply(bundle, []byte(key), recovery.ApplyOptions{Root: root})
						if applyErr != nil {
							err = applyErr
						} else {
							value.SecretsRestored, value.AppliedFiles = result.SecretsRestored, len(result.AppliedFiles)
							value.DatabaseRestored = result.DatabaseRestored
							value.AppdataRestored = append(value.AppdataRestored, result.AppdataRestored...)
							value.SharesRestored = append(value.SharesRestored, result.SharesRestored...)
							if result.DatabaseRestored != plan.DatabaseValid || len(result.AppdataRestored) != len(plan.Appdata) || len(result.SharesRestored) != len(plan.Shares) {
								err = errors.New("restore drill did not reproduce all database, appdata, and share payloads")
							}
							if err == nil {
								value.ServicesHealthy = true
								for _, name := range stackNamesFromRecoveryPlan(plan.Files) {
									ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
									rehearsal, rehearsalErr := s.dockerService.RehearseStack(ctx, root, name, 90*time.Second)
									cancel()
									if rehearsalErr != nil {
										value.ServicesHealthy = false
										err = fmt.Errorf("isolated service rehearsal %s failed: %w", name, rehearsalErr)
										break
									}
									value.ServicesRehearsed = append(value.ServicesRehearsed, rehearsal.Stack)
								}
							}
						}
					}
				}
			}
		}
	}
	if err != nil {
		value.State, value.Error = "failed", err.Error()
	} else {
		value.State = "successful"
	}
	now := time.Now().UTC()
	value.FinishedAt = &now
	_ = s.store.SaveRestoreDrill(value)
	severity := "info"
	if value.State == "failed" {
		severity = "critical"
	}
	s.publish("recovery.restore_drill."+value.State, severity, &model.ResourceRef{Type: "restore-drill", ID: value.ID}, map[string]any{"drill": value})
}

func stackNamesFromRecoveryPlan(files []string) []string {
	result := make([]string, 0)
	for _, name := range files {
		if strings.HasPrefix(name, "docker/stacks/") && strings.HasSuffix(name, "/compose.yaml") {
			stack := strings.TrimSuffix(strings.TrimPrefix(name, "docker/stacks/"), "/compose.yaml")
			if stack != "" && !strings.Contains(stack, "/") {
				result = append(result, stack)
			}
		}
	}
	return result
}

func (s *apiServer) restoreDrillLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		schedule, err := s.store.RestoreDrillSchedule()
		if err != nil || !schedule.Enabled {
			continue
		}
		now := time.Now().UTC()
		if schedule.NextDueAt == nil {
			next := now.Add(time.Duration(schedule.IntervalSeconds) * time.Second)
			schedule.NextDueAt = &next
			_, _ = s.store.SaveRestoreDrillSchedule(schedule)
			continue
		}
		if now.Before(*schedule.NextDueAt) {
			continue
		}
		if _, started := s.startRestoreDrill("scheduled", "system"); !started {
			continue
		}
		schedule.LastStartedAt = &now
		next := now.Add(time.Duration(schedule.IntervalSeconds) * time.Second)
		schedule.NextDueAt = &next
		_, _ = s.store.SaveRestoreDrillSchedule(schedule)
	}
}

func (s *apiServer) recoveryObjectiveAlerts(now time.Time) []model.Alert {
	s.recoveryObjectiveMu.Lock()
	defer s.recoveryObjectiveMu.Unlock()
	if now.Sub(s.recoveryObjectiveCheckedAt) < time.Minute {
		return append([]model.Alert(nil), s.recoveryObjectiveCached...)
	}
	schedule, err := s.store.RestoreDrillSchedule()
	if err != nil {
		return nil
	}
	alerts := make([]model.Alert, 0, 2)
	resource := &model.ResourceRef{Type: "backup", ID: "recovery"}
	key := s.recoveryKeyString()
	manifestFound := false
	var generation int64
	coveredWorkloads := map[string]bool{}
	if key != "" {
		path := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
		if bundle, readErr := os.ReadFile(path); readErr == nil {
			if plan, planErr := recovery.Plan(bundle, []byte(key)); planErr == nil {
				manifestFound = true
				generation = plan.Manifest.Generation
				for _, appdata := range plan.Appdata {
					coveredWorkloads[appdata.Stack] = true
				}
				age := now.Sub(plan.Manifest.CreatedAt)
				if age > time.Duration(schedule.RPOHours)*time.Hour {
					alerts = append(alerts, model.Alert{ID: "recovery-rpo-target", Severity: "warning", Title: "Recovery point is outside the RPO target", Description: fmt.Sprintf("Latest verified recovery point is %.1f hours old; target is %d hours", age.Hours(), schedule.RPOHours), Resource: resource, State: "firing", StartedAt: plan.Manifest.CreatedAt})
				}
				if age <= time.Duration(schedule.RPOHours)*time.Hour {
					_ = s.store.ClearAlertSuppression("recovery-rpo-target")
				}
			}
		}
	}
	if !manifestFound {
		alerts = append(alerts, model.Alert{ID: "recovery-rpo-target", Severity: "critical", Title: "No verified recovery point is available", Description: "Export and verify a recovery bundle to meet the configured RPO target", Resource: resource, State: "firing", StartedAt: now})
	}
	drills, _ := s.store.RestoreDrills(50)
	var latest *model.RestoreDrill
	for i := range drills {
		drill := &drills[i]
		if drill.State == "successful" && drill.ServicesHealthy && drill.FinishedAt != nil && manifestFound && drill.Generation == generation {
			latest = drill
			break
		}
	}
	if latest == nil {
		alerts = append(alerts, model.Alert{ID: "recovery-rto-unknown", Severity: "warning", Title: "Restore time has not been measured for the current bundle", Description: "Run a canary restore drill to measure recovery time against the RTO target", Resource: resource, State: "firing", StartedAt: now})
	} else if duration := latest.FinishedAt.Sub(latest.StartedAt); duration > time.Duration(schedule.RTOMinutes)*time.Minute {
		alerts = append(alerts, model.Alert{ID: "recovery-rto-target", Severity: "warning", Title: "Canary restore exceeded the RTO target", Description: fmt.Sprintf("Latest canary restore took %.1f minutes; target is %d minutes", duration.Minutes(), schedule.RTOMinutes), Resource: resource, State: "firing", StartedAt: *latest.FinishedAt})
	}
	if latest != nil && latest.FinishedAt.Sub(latest.StartedAt) <= time.Duration(schedule.RTOMinutes)*time.Minute {
		_ = s.store.ClearAlertSuppression("recovery-rto-unknown")
		_ = s.store.ClearAlertSuppression("recovery-rto-target")
	}
	objectives, _ := s.store.WorkloadRecoveryObjectives()
	for _, objective := range objectives {
		digest := sha256.Sum256([]byte(objective.WorkloadID))
		id := "workload-recovery-" + hex.EncodeToString(digest[:8])
		workloadResource := &model.ResourceRef{Type: "docker-stack", ID: objective.WorkloadID}
		if manifestFound && !coveredWorkloads[objective.WorkloadID] {
			alerts = append(alerts, model.Alert{ID: id + "-coverage", Severity: "critical", Title: "Workload is missing from recovery bundle", Description: fmt.Sprintf("%s has recovery objectives but no app-data archive in the latest verified bundle", objective.WorkloadID), Resource: workloadResource, State: "firing", StartedAt: now})
		} else if manifestFound {
			path := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
			if bundle, readErr := os.ReadFile(path); readErr == nil {
				if plan, planErr := recovery.Plan(bundle, []byte(key)); planErr == nil && now.Sub(plan.Manifest.CreatedAt) > time.Duration(objective.RPOHours)*time.Hour {
					alerts = append(alerts, model.Alert{ID: id + "-rpo", Severity: "warning", Title: "Workload recovery point is outside its RPO", Description: fmt.Sprintf("%s recovery point is %.1f hours old; target is %d hours", objective.WorkloadID, now.Sub(plan.Manifest.CreatedAt).Hours(), objective.RPOHours), Resource: workloadResource, State: "firing", StartedAt: plan.Manifest.CreatedAt})
				}
			}
		}
		var workloadDrill *model.RestoreDrill
		for i := range drills {
			candidate := &drills[i]
			if candidate.State == "successful" && candidate.ServicesHealthy && candidate.Generation == generation && candidate.FinishedAt != nil {
				for _, restored := range candidate.AppdataRestored {
					if strings.HasPrefix(restored, objective.WorkloadID+":") {
						workloadDrill = candidate
						break
					}
				}
				if workloadDrill != nil {
					break
				}
			}
		}
		if workloadDrill == nil {
			alerts = append(alerts, model.Alert{ID: id + "-rto", Severity: "warning", Title: "Workload restore time is not measured", Description: fmt.Sprintf("Run a restore drill that includes %s to measure its RTO", objective.WorkloadID), Resource: workloadResource, State: "firing", StartedAt: now})
		} else if duration := workloadDrill.FinishedAt.Sub(workloadDrill.StartedAt); duration > time.Duration(objective.RTOMinutes)*time.Minute {
			alerts = append(alerts, model.Alert{ID: id + "-rto", Severity: "warning", Title: "Workload restore exceeded its RTO", Description: fmt.Sprintf("%s restored in %.1f minutes; target is %d minutes", objective.WorkloadID, duration.Minutes(), objective.RTOMinutes), Resource: workloadResource, State: "firing", StartedAt: *workloadDrill.FinishedAt})
		} else {
			_ = s.store.ClearAlertSuppression(id + "-rto")
		}
		if manifestFound && coveredWorkloads[objective.WorkloadID] {
			_ = s.store.ClearAlertSuppression(id + "-coverage")
			_ = s.store.ClearAlertSuppression(id + "-rpo")
		}
	}
	s.recoveryObjectiveCheckedAt = now
	s.recoveryObjectiveCached = append([]model.Alert(nil), alerts...)
	return alerts
}

// jsonDecode keeps this file independent from the large API router helpers.
func jsonDecode(r *http.Request, target any) error { return json.NewDecoder(r.Body).Decode(target) }
