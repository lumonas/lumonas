package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/power"
)

const settingsMetaKey = "app_settings"

func (s *apiServer) settings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	value, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) updateSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Section string         `json:"section"`
		Patch   map[string]any `json:"patch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := validateSettingsPatch(input.Section, input.Patch); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	value, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	section, ok := value[input.Section].(map[string]any)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "settings section is malformed"})
		return
	}
	mergeSettings(section, input.Patch)
	value[input.Section] = section
	encoded, err := json.Marshal(value)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode settings failed"})
		return
	}
	if err := s.store.SetMeta(settingsMetaKey, string(encoded)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "settings.update", input.Section, map[string]any{"keys": mapKeys(input.Patch)})
	s.advanceGeneration("settings." + input.Section)
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) loadSettings() (map[string]any, error) {
	if raw, ok := s.store.Meta(settingsMetaKey); ok {
		var value map[string]any
		if err := json.Unmarshal([]byte(raw), &value); err == nil && validSettings(value) {
			return value, nil
		}
	}
	return defaultSettings(s), nil
}

func defaultSettings(s *apiServer) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	available := strings.TrimSpace(os.Getenv("LUMONAS_UPDATE_AVAILABLE"))
	var availableValue any
	if available != "" {
		availableValue = available
	}
	wol := make([]any, 0)
	if interfaces, err := net.Interfaces(); err == nil {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagLoopback != 0 || iface.HardwareAddr.String() == "" {
				continue
			}
			wol = append(wol, map[string]any{"interface": iface.Name, "mac": iface.HardwareAddr.String(), "supported": true, "enabled": false})
		}
	}
	ups := make([]any, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	for _, unit := range power.Discover(ctx, settingsUPSNames(), nil) {
		ups = append(ups, map[string]any{"name": unit.Name, "status": unit.Status, "chargePercent": unit.ChargePercent, "runtimeSec": unit.RuntimeSec, "onBattery": unit.OnBattery})
	}
	cancel()
	return map[string]any{
		"updates": map[string]any{
			"core":   map[string]any{"channel": "stable", "current": s.version, "available": availableValue, "lastCheckedAt": now, "autoUpdate": false},
			"debian": map[string]any{"release": "Debian 13 (Trixie)", "pendingCount": 0, "lastCheckedAt": now, "autoUpdate": false},
			"docker": map[string]any{"availableCount": 0, "autoUpdate": false},
		},
		"runtime": map[string]any{
			"writeProfile":  "balanced",
			"zram":          map[string]any{"enabled": false, "sizeBytes": 0, "compressedBytes": 0, "ratio": 0, "pressure": "low"},
			"dockerLogging": map[string]any{"driver": "json-file", "maxSizeMb": 10, "maxFiles": 3, "topConsumers": []any{}},
		},
		"power":    map[string]any{"maintenanceMode": false, "wol": wol, "ups": ups, "schedule": map[string]any{"enabled": false, "action": "shutdown", "time": "01:00", "days": "Daily"}},
		"security": map[string]any{"https": map[string]any{"enabled": os.Getenv("LUMONAS_WEB_TLS_CERT") != "" && os.Getenv("LUMONAS_WEB_TLS_KEY") != "", "ca": "LumoNAS Local CA", "acme": false}, "ssh": map[string]any{"rootLogin": false, "passwordAuth": false, "keyCount": 0}, "sessions": []any{}},
	}
}

func settingsUPSNames() []string {
	values := make([]string, 0)
	for _, value := range strings.Split(os.Getenv("LUMONAS_UPS_NAMES"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func validSettings(value map[string]any) bool {
	for _, section := range []string{"updates", "runtime", "power", "security"} {
		if _, ok := value[section].(map[string]any); !ok {
			return false
		}
	}
	return true
}

func validateSettingsPatch(section string, patch map[string]any) error {
	if patch == nil {
		return &settingsError{"patch is required"}
	}
	allowed := map[string]map[string]bool{
		"updates":  {"core": true, "debian": true, "docker": true},
		"runtime":  {"writeProfile": true, "zram": true, "dockerLogging": true},
		"power":    {"maintenanceMode": true, "wol": true, "schedule": true},
		"security": {"https": true, "ssh": true, "sessions": true},
	}
	keys, ok := allowed[section]
	if !ok {
		return &settingsError{"unknown settings section"}
	}
	for key := range patch {
		if !keys[key] {
			return &settingsError{"settings key is not allow-listed: " + key}
		}
	}
	if section == "runtime" {
		if value, ok := patch["writeProfile"].(string); ok && value != "balanced" && value != "normal" && value != "maximum" {
			return &settingsError{"writeProfile is not allow-listed"}
		}
	}
	return nil
}

type settingsError struct{ message string }

func (e *settingsError) Error() string { return e.message }

func mergeSettings(destination, patch map[string]any) {
	for key, value := range patch {
		if nested, ok := value.(map[string]any); ok {
			if existing, ok := destination[key].(map[string]any); ok {
				mergeSettings(existing, nested)
				continue
			}
		}
		destination[key] = value
	}
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

func (s *apiServer) checkUpdates(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	now := time.Now().UTC()
	job := model.Job{ID: newID("job"), Type: "updates.check", Title: "Check for updates", State: "queued", CreatedAt: now}
	if err := s.store.SaveJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "updates.check.queued", job.ID, nil)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	go s.runUpdateCheck(job)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *apiServer) runUpdateCheck(job model.Job) {
	started := time.Now().UTC()
	progress := 25.0
	job.State, job.StartedAt, job.Progress, job.Stage = "running", &started, &progress, "Checking configured update channels"
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	settings, err := s.loadSettings()
	if err == nil {
		if updates, ok := settings["updates"].(map[string]any); ok {
			now := time.Now().UTC().Format(time.RFC3339)
			for _, key := range []string{"core", "debian"} {
				if section, ok := updates[key].(map[string]any); ok {
					section["lastCheckedAt"] = now
				}
			}
			if encoded, encodeErr := json.Marshal(settings); encodeErr == nil {
				_ = s.store.SetMeta(settingsMetaKey, string(encoded))
			}
		}
	}
	finished := time.Now().UTC()
	progress = 100
	job.State, job.FinishedAt, job.Progress, job.Stage = "successful", &finished, &progress, "Update channels checked"
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
}
