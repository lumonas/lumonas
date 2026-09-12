package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/updates"
)

type updateApplyRequest struct {
	Manifest    updates.Manifest `json:"manifest"`
	Signature   string           `json:"signature"`
	PackagePath string           `json:"packagePath"`
	BackupPath  string           `json:"backupPath"`
}

type updateHealthRequest struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
	Reason  string `json:"reason"`
}

func (s *apiServer) updateManager() *updates.Manager {
	return &updates.Manager{Root: envOr("LUMONAS_UPDATE_ROOT", "/var/lib/lumonas/updates")}
}

func (s *apiServer) updatesStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	state, err := s.updateManager().Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *apiServer) applyUpdate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input updateApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	publicKey, err := updates.ParsePublicKey(os.Getenv("LUMONAS_UPDATE_PUBLIC_KEY"))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	signature, err := updates.ParseSignature(input.Signature)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.PackagePath) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "packagePath is required"})
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is required before installing an update"})
		return
	}
	backupPath := input.BackupPath
	if backupPath == "" {
		backupPath = filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	}
	if _, err := os.Stat(backupPath); err != nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "a verified recovery bundle is required before installing an update"})
		return
	}
	bundle, err := os.ReadFile(backupPath)
	if err != nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "pre-update recovery bundle could not be read"})
		return
	}
	if _, err := recovery.Verify(bundle, []byte(key)); err != nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "pre-update recovery verification failed"})
		return
	}
	state, err := s.updateManager().StageAndActivate(input.PackagePath, input.Manifest, signature, publicKey)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "update was not staged: " + err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "update.stage", input.Manifest.Version, map[string]any{"slot": state.PendingSlot})
	s.publish("update.staged", "warning", nil, map[string]any{"version": input.Manifest.Version, "slot": state.PendingSlot})
	writeJSON(w, http.StatusAccepted, state)
}

func (s *apiServer) rollbackUpdate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	reason := "manual rollback"
	if r.Body != nil && r.ContentLength != 0 {
		var input struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if strings.TrimSpace(input.Reason) != "" {
			reason = input.Reason
		}
	}
	state, err := s.updateManager().Rollback(reason)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "update.rollback", state.ActiveSlot, map[string]any{"reason": reason})
	s.publish("update.rolled_back", "critical", nil, map[string]any{"slot": state.ActiveSlot, "reason": reason})
	writeJSON(w, http.StatusOK, state)
}

func (s *apiServer) updateHealth(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input updateHealthRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	manager := s.updateManager()
	if !input.Healthy {
		state, err := manager.Rollback(input.Reason)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.recordIdentityAudit(actor, "update.health.rollback", state.ActiveSlot, map[string]any{"reason": input.Reason})
		s.publish("update.health_failed", "critical", nil, map[string]any{"slot": state.ActiveSlot, "reason": input.Reason})
		writeJSON(w, http.StatusOK, state)
		return
	}
	state, err := manager.MarkHealthy(input.Version)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "update.health.confirm", state.ActiveSlot, map[string]any{"version": state.ActiveVersion})
	s.publish("update.healthy", "info", nil, map[string]any{"version": state.ActiveVersion, "slot": state.ActiveSlot})
	writeJSON(w, http.StatusOK, state)
}
