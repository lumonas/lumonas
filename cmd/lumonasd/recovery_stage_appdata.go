package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lumonas/lumonas/internal/recovery"
)

func (s *apiServer) recoveryStageAppdata(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Stack           string `json:"stack"`
		Confirmed       bool   `json:"confirmed"`
		Reauthenticated bool   `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	input.Stack = strings.TrimSpace(input.Stack)
	if input.Stack == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stack is required"})
		return
	}
	if !input.Confirmed || !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "explicit confirmation and reauthentication are required"})
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LUMONAS_RECOVERY_KEY is not configured"})
		return
	}
	bundlePath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recovery bundle not found"})
		return
	}
	directory := envOr("LUMONAS_RECOVERY_STAGING_DIR", "/var/lib/lumonas/recovery/staged")
	result, err := recovery.StageAppdata(bundle, []byte(key), directory, input.Stack)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "app data staging failed: " + err.Error()})
		return
	}
	s.pruneRecoveryStages(directory, 3, result.Directory)
	s.publishActor(actor, "recovery.appdata.staged", "warning", nil, map[string]any{"stack": input.Stack, "directory": result.Directory, "files": len(result.Files), "generation": result.Manifest.Generation})
	writeJSON(w, http.StatusAccepted, map[string]any{"scope": "appdata", "stack": input.Stack, "directory": result.Directory, "files": result.Files, "verified": true, "generation": result.Manifest.Generation})
}
