package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
)

type onboardingCompleteRequest struct {
	ServerName string            `json:"serverName"`
	Roles      map[string]string `json:"roles"`
	Protection struct {
		SyncTime string `json:"syncTime"`
		ScrubDay string `json:"scrubDay"`
	} `json:"protection"`
	Recovery struct {
		AutoConfigBackup bool   `json:"autoConfigBackup"`
		Destination      string `json:"destination"`
		KeyAcknowledged  bool   `json:"keyAcknowledged"`
	} `json:"recovery"`
}

func (s *apiServer) onboardingState(w http.ResponseWriter, r *http.Request) {
	if s.authRequired {
		if _, ok := s.identityActor(w, r, false); !ok {
			return
		}
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk discovery unavailable"})
		return
	}
	name, _ := s.store.Meta("server_name")
	completed, _ := s.store.Meta("onboarding_complete")
	if name == "" {
		name = collector.Hostname()
	}
	items := make([]map[string]any, 0, len(disks))
	for _, disk := range disks {
		classification := classifyOnboardingDisk(disk)
		dataFound := classification == "existing" || classification == "lumonas" || classification == "suspected-parity"
		label := disk.Name
		if label == "" {
			label = disk.Model
		}
		items = append(items, map[string]any{"id": disk.ID, "name": disk.Name, "model": disk.Model, "serialSuffix": serialSuffix(disk.Serial), "sizeBytes": disk.SizeBytes, "classification": classification, "filesystem": disk.Filesystem, "dataFound": dataFound, "recommendedRole": recommendedOnboardingRole(disk, classification), "recommendedLabel": label, "offline": false})
	}
	metrics := collector.Metrics()
	parityAvailable := false
	for _, disk := range disks {
		if disk.Role == "parity" {
			parityAvailable = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"server":     map[string]any{"name": name, "hostname": collector.Hostname(), "timezone": timeZone(), "ip": primaryIP(), "sshEnabled": false},
		"hardware":   map[string]any{"cpu": fmt.Sprintf("%d logical CPUs", runtime.NumCPU()), "ramBytes": metrics.RAMTotalBytes, "diskCount": len(disks)},
		"disks":      items,
		"protection": map[string]any{"snapraidAvailable": true, "parityAvailable": parityAvailable},
		"recovery":   map[string]any{"keyConfigured": s.recoveryKeyString() != "", "latestVerified": false},
		"completed":  completed == "true",
	})
}

func (s *apiServer) completeOnboarding(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input onboardingCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	name := strings.TrimSpace(input.ServerName)
	if name == "" || len(name) > 63 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "serverName must be between 1 and 63 characters"})
		return
	}
	if input.Recovery.AutoConfigBackup && !input.Recovery.KeyAcknowledged {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "recovery key acknowledgement is required when automatic backups are enabled"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk discovery unavailable"})
		return
	}
	known := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		known[disk.ID] = disk
	}
	for diskID, role := range input.Roles {
		if _, ok := known[diskID]; !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "role references a disk that is no longer present: " + diskID})
			return
		}
		if !validOnboardingRole(role) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "unsupported disk role: " + role})
			return
		}
	}
	if input.Protection.SyncTime != "" {
		if _, _, err := monitoring.ParseTimeOfDay(input.Protection.SyncTime); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
	}
	if input.Protection.ScrubDay != "" {
		if _, err := monitoring.ParseWeekday(input.Protection.ScrubDay); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
	}
	if input.Recovery.AutoConfigBackup {
		if _, err := s.ensureOnboardingRecoveryKey(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recovery key setup failed: " + err.Error()})
			return
		}
	}
	if err := s.store.SetMeta("server_name", name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.SetMeta("onboarding_complete", "true"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	roles, _ := json.Marshal(input.Roles)
	_ = s.store.SetMeta("onboarding_roles", string(roles))
	protection, _ := json.Marshal(input.Protection)
	_ = s.store.SetMeta("onboarding_protection", string(protection))
	recovery, _ := json.Marshal(input.Recovery)
	_ = s.store.SetMeta("onboarding_recovery", string(recovery))
	s.recordIdentityAudit(actor, "onboarding.complete", "setup", map[string]any{"roles": len(input.Roles), "autoConfigBackup": input.Recovery.AutoConfigBackup})
	protectionConfigured := false
	if parityID, dataIDs := onboardingProtectionDisks(input.Roles); parityID != "" && len(dataIDs) > 0 {
		protectionConfigured = s.applySnapraidConfiguration(parityID, dataIDs)
	}
	s.applyOnboardingSchedules(input.Protection.SyncTime, input.Protection.ScrubDay)
	s.advanceGeneration("onboarding.complete")
	s.publish("onboarding.completed", "info", &model.ResourceRef{Type: "server", ID: "server-1"}, nil)
	initialSyncStarted := false
	if protectionConfigured {
		for diskID, role := range input.Roles {
			if role == "parity" && known[diskID].SizeBytes > 0 {
				job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Type: "snapraid.sync", Title: "snapraid sync", ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
				if err := s.store.SaveJob(job); err == nil {
					initialSyncStarted = true
					s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
					go s.runProtectionJob(job)
				}
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "initialSyncStarted": initialSyncStarted, "protectionConfigured": protectionConfigured})
}

// onboardingProtectionDisks extracts the SnapRAID layout from the assigned
// roles: one parity disk and every data disk become the protected set.
func onboardingProtectionDisks(roles map[string]string) (string, []string) {
	parity := ""
	data := make([]string, 0)
	for diskID, role := range roles {
		switch role {
		case "parity":
			if parity == "" {
				parity = diskID
			}
		case "data":
			data = append(data, diskID)
		}
	}
	return parity, data
}

func classifyOnboardingDisk(disk model.Disk) string {
	switch {
	case disk.Role == "system":
		return "system"
	case disk.Role == "parity":
		return "suspected-parity"
	case disk.Role == "external":
		return "removable"
	case disk.Role == "lumonas":
		return "lumonas"
	case disk.Filesystem == "":
		return "blank"
	default:
		return "existing"
	}
}

func recommendedOnboardingRole(disk model.Disk, classification string) string {
	if disk.Role != "" && disk.Role != "unknown" {
		return disk.Role
	}
	if classification == "system" {
		return "system"
	}
	return "data"
}

func serialSuffix(serial string) string {
	serial = strings.TrimSpace(serial)
	if len(serial) > 4 {
		return serial[len(serial)-4:]
	}
	return serial
}

func validOnboardingRole(role string) bool {
	switch role {
	case "system", "data", "parity", "apps", "backup", "external", "unknown", "":
		return true
	default:
		return false
	}
}

func (s *apiServer) ensureOnboardingRecoveryKey() (string, error) {
	if existing := s.recoveryKeyString(); existing != "" {
		return existing, nil
	}
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", err
	}
	key := hex.EncodeToString(keyBytes)
	directory := envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", err
	}
	path := envOr("LUMONAS_RECOVERY_KEY_FILE", filepath.Join(directory, "recovery.key"))
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", err
	}
	return key, nil
}

func (s *apiServer) createRecoveryKey(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	if completed, _ := s.store.Meta("onboarding_complete"); completed == "true" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "recovery key export is only available during onboarding"})
		return
	}
	key, err := s.ensureOnboardingRecoveryKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recovery key setup failed: " + err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "recovery.key.export", "recovery", map[string]any{"length": len(key)})
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "generated": true})
}

func timeZone() string {
	return "UTC"
}
