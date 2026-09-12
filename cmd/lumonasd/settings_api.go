package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/power"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/updates"
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
	s.markCurrentSession(value, r)
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) revokeSession(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteSessionByID(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "settings.session.revoke", id, nil)
	writeJSON(w, http.StatusNoContent, nil)
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
	if input.Section == "power" {
		if err := s.applyWOLSettings(r, input.Patch); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
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
	s.recordRequestAudit(r, actor, "settings.update", input.Section, map[string]any{"keys": mapKeys(input.Patch)})
	s.advanceGeneration("settings." + input.Section)
	writeJSON(w, http.StatusOK, value)
}

// applyWOLSettings turns the settings-panel toggle into a typed privileged
// operation. The full inventory is accepted because the UI sends the current
// list; every entry is validated before the first command is issued.
func (s *apiServer) applyWOLSettings(r *http.Request, patch map[string]any) error {
	raw, ok := patch["wol"]
	if !ok {
		return nil
	}
	entries, ok := raw.([]any)
	if !ok {
		return &settingsError{"power.wol must be an array"}
	}
	type change struct {
		iface   string
		enabled bool
	}
	changes := make([]change, 0, len(entries))
	for _, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			return &settingsError{"power.wol entries must be objects"}
		}
		iface, _ := entry["interface"].(string)
		supported, _ := entry["supported"].(bool)
		enabled, enabledOK := entry["enabled"].(bool)
		if strings.TrimSpace(iface) == "" || !enabledOK {
			return &settingsError{"power.wol entries require interface and enabled"}
		}
		if enabled && !supported {
			return &settingsError{"Wake-on-LAN is not supported on " + iface}
		}
		if !supported {
			continue
		}
		changes = append(changes, change{iface: iface, enabled: enabled})
	}
	for _, item := range changes {
		op := newID("wol")
		request := privileged.Request{
			Operation:      "network.wol.set",
			CorrelationID:  requestCorrelationID(r),
			OperationID:    op,
			PlanHash:       op,
			RequestedState: map[string]any{"interface": item.iface, "enabled": item.enabled},
			ExpiresAt:      time.Now().UTC().Add(2 * time.Minute),
			Confirmed:      true,
		}
		if err := s.brokerExecute(r.Context(), request); err != nil {
			return fmt.Errorf("apply Wake-on-LAN for %s: %w", item.iface, err)
		}
	}
	return nil
}

func (s *apiServer) loadSettings() (map[string]any, error) {
	var value map[string]any
	if raw, ok := s.store.Meta(settingsMetaKey); ok {
		var stored map[string]any
		if err := json.Unmarshal([]byte(raw), &stored); err == nil && validSettings(stored) {
			value = stored
		}
	}
	if value == nil {
		value = defaultSettings(s)
	}
	s.refreshDynamicSettings(value)
	return value, nil
}

// refreshDynamicSettings overwrites inventory-backed settings values that go
// stale when persisted (docker update counts, ssh key count, live sessions).
func (s *apiServer) refreshDynamicSettings(value map[string]any) {
	if updates, ok := value["updates"].(map[string]any); ok {
		if docker, ok := updates["docker"].(map[string]any); ok {
			_, _, updateCount := s.dockerCounts()
			docker["availableCount"] = updateCount
		}
	}
	if security, ok := value["security"].(map[string]any); ok {
		if ssh, ok := security["ssh"].(map[string]any); ok {
			keys, err := readSSHKeys()
			if err == nil {
				ssh["keyCount"] = len(keys)
			}
		}
		security["sessions"] = s.sessionViews("")
	}
}

// sessionViews renders active sessions for the settings security panel. The
// session whose cookie digest matches currentDigest is flagged as current.
func (s *apiServer) sessionViews(currentDigest string) []map[string]any {
	sessions, err := s.store.Sessions()
	if err != nil {
		return []map[string]any{}
	}
	views := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		views = append(views, map[string]any{
			"id":           session.ID,
			"device":       session.Username,
			"ip":           "local",
			"scope":        "management",
			"lastActiveAt": session.CreatedAt.Format(time.RFC3339),
			"expiresAt":    session.ExpiresAt.Format(time.RFC3339),
			"current":      currentDigest != "" && session.ID == currentDigest,
		})
	}
	return views
}

func (s *apiServer) markCurrentSession(value map[string]any, r *http.Request) {
	cookie, err := r.Cookie("lumonas_session")
	if err != nil {
		return
	}
	digest := auth.TokenDigest(cookie.Value)
	security, ok := value["security"].(map[string]any)
	if !ok {
		return
	}
	sessions, ok := security["sessions"].([]map[string]any)
	if !ok {
		return
	}
	for _, session := range sessions {
		if session["id"] == digest {
			session["current"] = true
		} else if current, isBool := session["current"].(bool); isBool && current {
			session["current"] = false
		}
	}
}

func defaultSettings(s *apiServer) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	available := strings.TrimSpace(os.Getenv("LUMONAS_UPDATE_AVAILABLE"))
	var availableValue any
	if available != "" {
		availableValue = available
	}
	wol := make([]any, 0)
	ctxWOL, cancelWOL := context.WithTimeout(context.Background(), 5*time.Second)
	for _, iface := range network.DiscoverWOL(ctxWOL, nil) {
		wol = append(wol, iface)
	}
	cancelWOL()
	ups := make([]any, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	for _, unit := range power.Discover(ctx, settingsUPSNames(), nil) {
		ups = append(ups, map[string]any{"name": unit.Name, "status": unit.Status, "chargePercent": unit.ChargePercent, "runtimeSec": unit.RuntimeSec, "onBattery": unit.OnBattery})
	}
	cancel()
	keyCount := 0
	if keys, err := readSSHKeys(); err == nil {
		keyCount = len(keys)
	}
	return map[string]any{
		"updates": map[string]any{
			"core":   map[string]any{"channel": "stable", "current": s.version, "available": availableValue, "lastCheckedAt": now, "autoUpdate": false, "channelUrl": strings.TrimSpace(os.Getenv("LUMONAS_UPDATE_FEED_URL"))},
			"debian": map[string]any{"release": "Debian 13 (Trixie)", "pendingCount": 0, "lastCheckedAt": now, "autoUpdate": false},
			"docker": map[string]any{"availableCount": 0, "autoUpdate": false},
		},
		"runtime": map[string]any{
			"writeProfile":  "balanced",
			"zram":          map[string]any{"enabled": false, "sizeBytes": 0, "compressedBytes": 0, "ratio": 0, "pressure": "low"},
			"dockerLogging": map[string]any{"driver": "json-file", "maxSizeMb": 10, "maxFiles": 3, "topConsumers": []any{}},
		},
		"power":    map[string]any{"maintenanceMode": false, "wol": wol, "ups": ups, "schedule": map[string]any{"enabled": false, "action": "shutdown", "time": "01:00", "days": "Daily"}},
		"security": map[string]any{"https": map[string]any{"enabled": os.Getenv("LUMONAS_WEB_TLS_CERT") != "" && os.Getenv("LUMONAS_WEB_TLS_KEY") != "", "ca": "LumoNAS Local CA", "acme": false}, "ssh": map[string]any{"rootLogin": false, "passwordAuth": false, "keyCount": keyCount}, "sessions": []any{}},
	}
}

// dockerCounts reports stack/running/update numbers from the Docker service.
// Zero values are reported when Docker is unavailable or unconfigured (tests).
func (s *apiServer) dockerCounts() (stacks, running, updates int) {
	if s.dockerService.Run == nil {
		return 0, 0, 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stackList, _ := s.dockerService.Stacks(ctx)
	containers, _ := s.dockerService.Containers(ctx)
	images, _ := s.dockerService.Images(ctx)
	for _, container := range containers {
		if container.State == "running" || container.State == "restarting" {
			running++
		}
	}
	for _, image := range images {
		if image.UpdateAvailable {
			updates++
		}
	}
	return len(stackList), running, updates
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
	job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Type: "updates.check", Title: "Check for updates", State: "queued", CreatedAt: now}
	if err := s.store.SaveJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "updates.check.queued", job.ID, map[string]any{"jobId": job.ID})
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	go func() { _ = s.runUpdateCheck(job) }()
	writeJSON(w, http.StatusAccepted, job)
}

func (s *apiServer) runUpdateCheck(job model.Job) model.Job {
	started := time.Now().UTC()
	progress := 25.0
	job.State, job.StartedAt, job.Progress, job.Stage = "running", &started, &progress, "Checking configured update channels"
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	stage := "Update channels checked"
	if settings, err := s.loadSettings(); err == nil {
		if updates, ok := settings["updates"].(map[string]any); ok {
			now := time.Now().UTC().Format(time.RFC3339)
			for _, key := range []string{"core", "debian"} {
				if section, ok := updates[key].(map[string]any); ok {
					section["lastCheckedAt"] = now
				}
			}
			core, _ := updates["core"].(map[string]any)
			if feedErr := s.checkUpdateFeed(core); feedErr != nil {
				core["lastError"] = feedErr.Error()
				stage = "Feed check failed — kept last known state"
				s.publish("updates.check.failed", "warning", nil, map[string]any{"jobId": job.ID, "error": feedErr.Error()})
			} else {
				delete(core, "lastError")
			}
			if encoded, encodeErr := json.Marshal(settings); encodeErr == nil {
				_ = s.store.SetMeta(settingsMetaKey, string(encoded))
			}
		}
	}
	finished := time.Now().UTC()
	progress = 100
	job.State, job.FinishedAt, job.Progress, job.Stage = "successful", &finished, &progress, stage
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	return job
}

// checkUpdateFeed polls the configured signed update feed and refreshes the
// advertised availability. A missing feed URL is a valid offline posture, not
// an error; verification or fetch failures keep the last known state.
func (s *apiServer) checkUpdateFeed(core map[string]any) error {
	channelURL := ""
	if value, ok := core["channelUrl"].(string); ok {
		channelURL = strings.TrimSpace(value)
	}
	if channelURL == "" {
		return nil
	}
	publicKey, keyErr := updates.ParsePublicKey(os.Getenv("LUMONAS_UPDATE_PUBLIC_KEY"))
	if keyErr != nil {
		return keyErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	document, err := updates.FetchFeed(ctx, s.updateHTTPClient, channelURL)
	if err != nil {
		return err
	}
	manifest, err := updates.VerifyFeedDocument(publicKey, document)
	if err != nil {
		return err
	}
	current := s.version
	if value, ok := core["current"].(string); ok && value != "" {
		current = value
	}
	if updates.CompareVersions(manifest.Version, current) > 0 {
		core["available"] = manifest.Version
		core["releaseNotes"] = manifest.Notes
		core["publishedAt"] = manifest.PublishedAt.Format(time.RFC3339)
	} else {
		delete(core, "available")
		delete(core, "releaseNotes")
		delete(core, "publishedAt")
	}
	return nil
}
