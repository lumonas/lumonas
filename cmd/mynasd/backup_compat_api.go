package main

import (
	"net/http"
	"time"

	"github.com/lumonas/lumonas/internal/model"
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
	layers := []map[string]any{
		{"id": "recovery-key", "label": "Recovery key", "status": readinessStatus(keyReady), "detail": readinessDetail(keyReady, "Configured", "Not configured")},
		{"id": "destinations", "label": "Backup destinations", "status": readinessStatus(len(destinations) > 0), "detail": readinessDetail(len(destinations) > 0, "Configured", "No destination configured")},
		{"id": "latest-bundle", "label": "Latest verified bundle", "status": readinessStatus(latestReady), "detail": readinessDetail(latestReady, "Verified", "No verified bundle")},
	}
	score := 0
	for _, layer := range layers {
		if layer["status"] == "current" {
			score += 100 / len(layers)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"score": score, "layers": layers})
}

func (s *apiServer) backupJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	runs, err := s.store.BackupRuns(50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	values := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		status := model.Healthy
		if run.State == "failed" {
			status = model.Warning
		}
		var lastRun map[string]any
		if run.FinishedAt != nil {
			lastRun = map[string]any{"status": status, "at": run.FinishedAt, "detail": run.Error}
		}
		values = append(values, map[string]any{"id": run.ID, "name": "Recovery bundle", "source": "configuration", "destinationId": "", "schedule": run.Trigger, "strategy": "verified snapshot", "jobType": "recovery.bundle", "lastRun": lastRun, "enabled": true})
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) runBackupJob(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	runs, err := s.store.BackupRuns(500)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, run := range runs {
		if run.ID != id {
			continue
		}
		run.Trigger = "manual"
		run.State = "queued"
		run.StartedAt = time.Now().UTC()
		if err := s.store.SaveBackupRun(run); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.recordIdentityAudit(actor, "backup.run.request", run.ID, nil)
		go s.executeBackup(run)
		writeJSON(w, http.StatusAccepted, run)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "backup job not found"})
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
	s.recoveryPlan(w)
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
