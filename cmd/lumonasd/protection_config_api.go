package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

func (s *apiServer) snapraidConfigPath() string {
	return envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf")
}

// protectionConfig returns the currently configured protection layout as
// stable disk identities. Configuration with missing disks still reports
// its configured members so the UI can explain what is absent.
func (s *apiServer) protectionConfig(w http.ResponseWriter, r *http.Request) {
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	protection := storage.DiscoverProtection(ctx, disks, nil, s.snapraidConfigPath())
	parityIDs := make([]string, 0, len(protection.ParityDisks))
	for _, parity := range protection.ParityDisks {
		parityIDs = append(parityIDs, parity.DiskID)
	}
	configured := true
	if _, readErr := os.ReadFile(s.snapraidConfigPath()); readErr != nil {
		configured = false
	}
	writeJSON(w, http.StatusOK, map[string]any{"configPath": s.snapraidConfigPath(), "configured": configured, "parityDiskIds": parityIDs, "dataDiskIds": protection.ProtectedDiskIDs})
}

func (s *apiServer) updateProtectionConfig(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ParityDiskID string   `json:"parityDiskId"`
		DataDiskIDs  []string `json:"dataDiskIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	present := make(map[string]bool, len(disks))
	for _, disk := range disks {
		present[disk.ID] = true
	}
	if len(input.DataDiskIDs) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "dataDiskIds must contain at least one stable disk identity"})
		return
	}
	for _, id := range append(append([]string{}, input.DataDiskIDs...), input.ParityDiskID) {
		if id != "" && !present[id] {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "diskId is not a currently discovered stable identity: " + id})
			return
		}
	}
	expected, err := expectedSnapraidDisks(disks, input.ParityDiskID, input.DataDiskIDs)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	rendered, err := storage.RenderSnapraidConfig(input.ParityDiskID, input.DataDiskIDs)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	request := privileged.Request{Operation: "snapraid.config.apply", OperationID: newID("snapraid-config"), PlanHash: snapraidPlanHash(rendered), ExpectedDisks: expected, RequestedState: map[string]any{"configPath": s.snapraidConfigPath(), "parityDiskId": input.ParityDiskID, "dataDiskIds": stringsToAny(input.DataDiskIDs)}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Confirmed: true}
	result, err := (privileged.Client{Socket: envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")}).Execute(r.Context(), request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.advanceGeneration("storage.protection.config")
	s.publish("storage.protection.configured", "warning", &model.ResourceRef{Type: "pool", ID: "protection"}, map[string]any{"parityDiskId": input.ParityDiskID, "dataDiskIds": input.DataDiskIDs})
	writeJSON(w, http.StatusOK, map[string]any{"configPath": s.snapraidConfigPath(), "parityDiskId": input.ParityDiskID, "dataDiskIds": input.DataDiskIDs, "config": rendered})
}

// applySnapraidConfiguration generates and activates the managed snapraid
// config through the privileged broker. It is best-effort: onboarding and
// role changes must not fail when the broker is unavailable, but a failure
// is reported so the caller can surface it.
func (s *apiServer) applySnapraidConfiguration(parityDiskID string, dataDiskIDs []string) bool {
	disks, err := s.diskFunc()
	if err != nil {
		s.warnProtectionConfig(err)
		return false
	}
	expected, err := expectedSnapraidDisks(disks, parityDiskID, dataDiskIDs)
	if err != nil {
		s.warnProtectionConfig(err)
		return false
	}
	rendered, err := storage.RenderSnapraidConfig(parityDiskID, dataDiskIDs)
	if err != nil {
		s.warnProtectionConfig(err)
		return false
	}
	request := privileged.Request{Operation: "snapraid.config.apply", OperationID: newID("snapraid-config"), PlanHash: snapraidPlanHash(rendered), ExpectedDisks: expected, RequestedState: map[string]any{"configPath": s.snapraidConfigPath(), "parityDiskId": parityDiskID, "dataDiskIds": stringsToAny(dataDiskIDs)}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Confirmed: true}
	result, err := (privileged.Client{Socket: envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")}).Execute(context.Background(), request)
	if err != nil {
		s.warnProtectionConfig(err)
		return false
	}
	if !result.OK {
		s.warnProtectionConfig(fmt.Errorf("%s", result.Error))
		return false
	}
	return true
}

func (s *apiServer) warnProtectionConfig(err error) {
	if s.log != nil {
		s.log.Warn("SnapRAID configuration could not be applied", "error", err)
	}
}

// applyOnboardingSchedules translates the onboarding protection preferences
// into the persisted sync/scrub schedules so the stored choices actually run.
func (s *apiServer) applyOnboardingSchedules(syncTime, scrubDay string) {
	schedules, err := s.store.JobSchedules(time.Now())
	if err != nil {
		s.warnProtectionConfig(err)
		return
	}
	for _, schedule := range schedules {
		changed := false
		switch schedule.ID {
		case "sched-sync":
			if syncTime != "" {
				if _, _, err := monitoring.ParseTimeOfDay(syncTime); err == nil {
					schedule.TimeOfDay = syncTime
					changed = true
				}
			}
		case "sched-scrub":
			if scrubDay != "" {
				if _, err := monitoring.ParseWeekday(scrubDay); err == nil {
					schedule.Weekday = strings.ToLower(scrubDay)
					changed = true
				}
			}
		default:
			continue
		}
		if changed {
			now := time.Now()
			next := monitoring.NextOccurrence(schedule, now)
			schedule.NextDueAt = &next
			if err := s.store.SaveJobSchedule(schedule); err != nil {
				s.warnProtectionConfig(err)
			}
		}
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func expectedSnapraidDisks(disks []model.Disk, parityID string, dataIDs []string) ([]privileged.ExpectedDisk, error) {
	ids := append([]string{}, dataIDs...)
	if parityID != "" {
		ids = append(ids, parityID)
	}
	byID := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	result := make([]privileged.ExpectedDisk, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		disk, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("disk identity is no longer present: %s", id)
		}
		seen[id] = true
		result = append(result, privileged.ExpectedDisk{ID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, GPTDiskGUID: disk.GPTDiskGUID, PartitionUUID: disk.PartitionUUID, FilesystemUUID: disk.FilesystemUUID})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one stable SnapRAID disk identity is required")
	}
	return result, nil
}

func snapraidPlanHash(config string) string {
	return fmt.Sprintf("snapraid-config-%x", sha256.Sum256([]byte(config)))
}
