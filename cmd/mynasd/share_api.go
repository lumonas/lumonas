package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lumonas/lumonas/internal/shares"
)

type sharePayload struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Path               string          `json:"path"`
	Description        string          `json:"description"`
	Enabled            bool            `json:"enabled"`
	Guest              bool            `json:"guest"`
	Protocols          json.RawMessage `json:"protocols"`
	Access             json.RawMessage `json:"access"`
	ExpectedGeneration *int64          `json:"expectedGeneration"`
}

func decodeManagedShare(r *http.Request) (shares.ManagedShare, *int64, error) {
	var payload sharePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return shares.ManagedShare{}, nil, errors.New("invalid JSON")
	}
	protocols, err := decodeProtocols(payload.Protocols)
	if err != nil {
		return shares.ManagedShare{}, nil, err
	}
	access, err := decodeAccess(payload.Access)
	if err != nil {
		return shares.ManagedShare{}, nil, err
	}
	return shares.ManagedShare{ID: payload.ID, Name: payload.Name, Path: payload.Path, Description: payload.Description, Enabled: payload.Enabled, Guest: payload.Guest, Protocols: protocols, Access: access}, payload.ExpectedGeneration, nil
}

func decodeProtocols(raw json.RawMessage) ([]shares.Protocol, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("at least one share protocol is required")
	}
	var detailed []shares.Protocol
	if err := json.Unmarshal(raw, &detailed); err == nil {
		return detailed, nil
	}
	var legacy []string
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, errors.New("protocols must be an array")
	}
	result := make([]shares.Protocol, 0, len(legacy))
	for _, name := range legacy {
		result = append(result, shares.Protocol{Name: name})
	}
	return result, nil
}

func decodeAccess(raw json.RawMessage) ([]shares.AccessRule, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var detailed []shares.AccessRule
	if err := json.Unmarshal(raw, &detailed); err == nil {
		return detailed, nil
	}
	var legacy map[string]string
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, errors.New("access must be an object or array")
	}
	result := make([]shares.AccessRule, 0, len(legacy))
	for name, level := range legacy {
		result = append(result, shares.AccessRule{PrincipalName: name, Level: level})
	}
	return result, nil
}

func (s *apiServer) listManagedShares(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	if err := s.store.ImportLegacyShares(s.shareStore().Path); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "legacy share import failed: " + err.Error()})
		return
	}
	values, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) createManagedShare(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	if err := s.store.ImportLegacyShares(s.shareStore().Path); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "legacy share import failed: " + err.Error()})
		return
	}
	share, expectedGeneration, err := decodeManagedShare(r)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if !s.expectedIdentityGeneration(w, expectedGeneration) {
		return
	}
	if share.ID == "" {
		share.ID = newID("share")
	}
	existing, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, current := range existing {
		if current.Name == share.Name {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "share name already exists"})
			return
		}
	}
	candidate := append(append([]shares.ManagedShare(nil), existing...), share)
	temporary, config, err := s.prepareSambaConfig(candidate)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.store.CreateManagedShare(share)
	if err != nil {
		_ = os.Remove(temporary)
		writeJSON(w, statusForShareError(err), map[string]string{"error": err.Error()})
		return
	}
	if err := os.Rename(temporary, config); err != nil {
		_ = os.Remove(temporary)
		_ = s.store.DeleteManagedShare(created.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate Samba configuration: " + err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "share.create", created.ID, map[string]any{"name": created.Name})
	s.advanceGeneration("share.create")
	writeJSON(w, http.StatusCreated, created)
}

func (s *apiServer) updateManagedShare(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	share, expectedGeneration, err := decodeManagedShare(r)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if !s.expectedIdentityGeneration(w, expectedGeneration) {
		return
	}
	share.ID = id
	existing, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	found := false
	var previous shares.ManagedShare
	for index := range existing {
		if existing[index].ID == id {
			previous = existing[index]
			existing[index] = share
			found = true
		} else if existing[index].Name == share.Name {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "share name already exists"})
			return
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	temporary, config, err := s.prepareSambaConfig(existing)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	updated, err := s.store.UpdateManagedShare(share)
	if err != nil {
		_ = os.Remove(temporary)
		writeJSON(w, statusForShareError(err), map[string]string{"error": err.Error()})
		return
	}
	if err := os.Rename(temporary, config); err != nil {
		_ = os.Remove(temporary)
		_, _ = s.store.UpdateManagedShare(previous)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate Samba configuration: " + err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "share.update", updated.ID, map[string]any{"name": updated.Name})
	s.advanceGeneration("share.update")
	writeJSON(w, http.StatusOK, updated)
}

func (s *apiServer) deleteManagedShare(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	existing, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	filtered := make([]shares.ManagedShare, 0, len(existing))
	found := false
	for _, share := range existing {
		if share.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, share)
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	temporary, config, err := s.prepareSambaConfig(filtered)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.DeleteManagedShare(id); err != nil {
		_ = os.Remove(temporary)
		writeJSON(w, statusForShareError(err), map[string]string{"error": err.Error()})
		return
	}
	if err := os.Rename(temporary, config); err != nil {
		_ = os.Remove(temporary)
		_, _ = s.store.CreateManagedShare(existingShareByID(existing, id))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate Samba configuration: " + err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "share.delete", id, nil)
	s.advanceGeneration("share.delete")
	w.WriteHeader(http.StatusNoContent)
}

func existingShareByID(values []shares.ManagedShare, id string) shares.ManagedShare {
	for _, value := range values {
		if value.ID == id {
			return value
		}
	}
	return shares.ManagedShare{}
}

func (s *apiServer) prepareSambaConfig(values []shares.ManagedShare) (string, string, error) {
	legacy := make([]shares.Share, 0, len(values))
	for _, value := range values {
		legacy = append(legacy, value.Legacy())
	}
	config, err := shares.RenderSamba(legacy)
	if err != nil {
		return "", "", err
	}
	configPath := envOr("MYNAS_SAMBA_CONFIG", "/var/lib/mynas/generated/smb.conf")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return "", "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(configPath), ".smb-validated-*.conf")
	if err != nil {
		return "", "", err
	}
	temporaryPath := temporary.Name()
	cleanup := func() { temporary.Close(); _ = os.Remove(temporaryPath) }
	if _, err := temporary.WriteString(config); err != nil {
		cleanup()
		return "", "", err
	}
	if err := temporary.Chmod(0o640); err != nil {
		cleanup()
		return "", "", err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return "", "", err
	}
	if err := shares.ValidateSamba(temporaryPath); err != nil {
		_ = os.Remove(temporaryPath)
		return "", "", fmt.Errorf("Samba configuration validation failed: %w", err)
	}
	return temporaryPath, configPath, nil
}

func statusForShareError(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	if strings.Contains(err.Error(), "already exists") {
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}
