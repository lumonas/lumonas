package main

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/power"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/store"
)

func (s *apiServer) schedules(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.JobSchedules(time.Now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) updateSchedule(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	schedule, err := s.store.JobSchedule(id, time.Now())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}
	if schedule.Kind == monitoring.ScheduleEvent {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "event-driven schedules cannot be edited"})
		return
	}
	var input struct {
		Enabled   *bool   `json:"enabled"`
		TimeOfDay *string `json:"timeOfDay"`
		Weekday   *string `json:"weekday"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.Enabled == nil && input.TimeOfDay == nil && input.Weekday == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled, timeOfDay, or weekday is required"})
		return
	}
	if input.Enabled != nil {
		schedule.Enabled = *input.Enabled
	}
	if input.TimeOfDay != nil {
		schedule.TimeOfDay = *input.TimeOfDay
	}
	if input.Weekday != nil {
		schedule.Weekday = strings.ToLower(*input.Weekday)
	}
	// A cadence change re-arms the schedule from now; enabling a paused
	// schedule keeps its previously computed due time when it is still future.
	if input.TimeOfDay != nil || input.Weekday != nil {
		now := time.Now()
		next := monitoring.NextOccurrence(schedule, now)
		schedule.NextDueAt = &next
	}
	if err := s.store.SaveJobSchedule(schedule); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "schedule.update", schedule.ID, map[string]any{"enabled": schedule.Enabled, "timeOfDay": schedule.TimeOfDay, "weekday": schedule.Weekday})
	s.advanceGeneration("schedule.update")
	s.publish("schedule.updated", "info", &model.ResourceRef{Type: "schedule", ID: schedule.ID}, map[string]any{"scheduleId": schedule.ID, "enabled": schedule.Enabled, "timeOfDay": schedule.TimeOfDay, "weekday": schedule.Weekday})
	saved, err := s.store.JobSchedule(schedule.ID, time.Now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// scheduleLoop drives persisted schedules; it mirrors backupLoop's one-minute tick.
// The tick counter also paces periodic alert-rule evaluation.
func (s *apiServer) scheduleLoop() {
	var tick int64
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		tick++
		s.runDueSchedules()
		s.runScheduledPower()
		s.evaluatePeriodicAlerts(tick)
		s.cleanupExpiredCSRFTokens()
	}
}

func (s *apiServer) runScheduledPower() {
	now := time.Now()
	if s.clock != nil {
		now = s.clock()
	}
	settings, err := s.loadSettings()
	if err != nil {
		return
	}
	powerSettings, ok := settings["power"].(map[string]any)
	if !ok {
		return
	}
	if maintenance, _ := powerSettings["maintenanceMode"].(bool); maintenance {
		return
	}
	raw, ok := powerSettings["schedule"].(map[string]any)
	if !ok {
		return
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return
	}
	var schedule power.Schedule
	if json.Unmarshal(encoded, &schedule) != nil || !schedule.Due(now) {
		return
	}
	minute := now.In(time.Local).Format("2006-01-02T15:04")
	if previous, ok := s.store.Meta("power_schedule_last_attempt"); ok && previous == minute {
		return
	}
	// Record the attempt before invoking the broker so a slow or failing
	// shutdown cannot be duplicated by the next scheduler tick.
	if err := s.store.SetMeta("power_schedule_last_attempt", minute); err != nil {
		return
	}
	operationID := newID("scheduled-power")
	resultErr := s.brokerExecute(context.Background(), privileged.Request{
		Operation:      "power.shutdown",
		CorrelationID:  "schedule-power-" + minute,
		OperationID:    operationID,
		PlanHash:       operationID,
		RequestedState: map[string]any{"action": schedule.Action},
		ExpiresAt:      now.UTC().Add(2 * time.Minute),
		Confirmed:      true,
	})
	if resultErr != nil {
		s.publish("power.schedule.failed", "warning", nil, map[string]any{"operationId": operationID, "action": schedule.Action, "error": resultErr.Error()})
		return
	}
	_ = s.store.SaveAudit(store.AuditEntry{Actor: "system", Action: "power.schedule", Outcome: "committed", Generation: s.currentGeneration(), ResourceType: "power", ResourceID: operationID, Metadata: map[string]any{"action": schedule.Action, "minute": minute, "correlationId": "schedule-power-" + minute}})
	s.publish("power.schedule.started", "critical", nil, map[string]any{"operationId": operationID, "action": schedule.Action})
}

func (s *apiServer) runDueSchedules() {
	schedules, err := s.store.JobSchedules(time.Now())
	if err != nil {
		if s.log != nil {
			s.log.Warn("schedule lookup failed", "error", err)
		}
		return
	}
	now := time.Now()
	for i := range schedules {
		schedule := schedules[i]
		if !schedule.Enabled || schedule.Kind == monitoring.ScheduleEvent {
			continue
		}
		if schedule.NextDueAt == nil {
			next := monitoring.NextOccurrence(schedule, now)
			if next.IsZero() {
				continue
			}
			schedule.NextDueAt = &next
			_ = s.store.SaveJobSchedule(schedule)
			continue
		}
		if now.Before(*schedule.NextDueAt) {
			continue
		}
		if s.jobTypeActive(schedule.JobType) {
			// Collision guard: re-arm instead of queueing a duplicate; the
			// next occurrence runs once the active job drains.
			next := monitoring.NextOccurrence(schedule, now)
			schedule.NextDueAt = &next
			_ = s.store.SaveJobSchedule(schedule)
			s.publish("schedule.skipped", "attention", &model.ResourceRef{Type: "schedule", ID: schedule.ID}, map[string]any{"scheduleId": schedule.ID, "reason": "job of the same type is already active"})
			continue
		}
		// Persist bookkeeping before launching so a crash between the two
		// steps cannot double-fire the schedule.
		schedule.MarkStarted(now)
		if err := s.store.SaveJobSchedule(schedule); err != nil {
			continue
		}
		s.launchScheduleJob(schedule)
	}
}

func (s *apiServer) jobTypeActive(jobType string) bool {
	jobs, err := s.store.Jobs()
	if err != nil {
		return true
	}
	for _, job := range jobs {
		if job.Type == jobType && jobIsActive(job.State) {
			return true
		}
	}
	return false
}

func (s *apiServer) launchScheduleJob(schedule monitoring.Schedule) {
	switch schedule.JobType {
	case "snapraid.sync", "snapraid.scrub":
		job := model.Job{ID: newID("job"), CorrelationID: "schedule-" + schedule.ID, Type: schedule.JobType, Title: strings.ReplaceAll(schedule.JobType, ".", " ") + " (scheduled)", ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
		if err := s.admitJob(job); err != nil {
			s.scheduleLaunchFailed(schedule, err)
			return
		}
		s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "schedule": schedule.ID})
		go s.runProtectionJob(job)
	case "smart.short", "smart.extended":
		disks, err := s.diskFunc()
		if err != nil || len(disks) == 0 {
			reason := "no disks discovered"
			if err != nil {
				reason = err.Error()
			}
			s.publish("schedule.skipped", "warning", &model.ResourceRef{Type: "schedule", ID: schedule.ID}, map[string]any{"scheduleId": schedule.ID, "reason": reason})
			return
		}
		for _, disk := range disks {
			job := model.Job{ID: newID("job"), CorrelationID: "schedule-" + schedule.ID, Type: schedule.JobType, Title: "SMART " + strings.TrimPrefix(schedule.JobType, "smart.") + " validation (scheduled)", ResourceID: disk.ID, State: "queued", CreatedAt: time.Now().UTC()}
			if err := s.admitJob(job); err != nil {
				s.scheduleLaunchFailed(schedule, err)
				continue
			}
			s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "schedule": schedule.ID})
			target := disk
			go s.runReadOnlyJob(job, target)
		}
	case "backup.run":
		s.requestAutomaticBackup("scheduled")
	default:
		s.publish("schedule.skipped", "warning", &model.ResourceRef{Type: "schedule", ID: schedule.ID}, map[string]any{"scheduleId": schedule.ID, "reason": "unsupported job type " + schedule.JobType})
	}
}

func (s *apiServer) scheduleLaunchFailed(schedule monitoring.Schedule, err error) {
	if s.log != nil {
		s.log.Warn("scheduled job launch failed", "schedule", schedule.ID, "error", err)
	}
	s.publish("schedule.failed", "warning", &model.ResourceRef{Type: "schedule", ID: schedule.ID}, map[string]any{"scheduleId": schedule.ID, "error": err.Error()})
}

func (s *apiServer) recordSystemAudit(action, id string, metadata map[string]any) {
	_ = s.store.SaveAudit(store.AuditEntry{
		Actor:        "system",
		Action:       action,
		Outcome:      "committed",
		ResourceType: "system",
		ResourceID:   id,
		Generation:   s.currentGeneration(),
		Metadata:     metadata,
	})
}

func scheduleID(endpoint string) string {
	return path.Base(endpoint)
}
