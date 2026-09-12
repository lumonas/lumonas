package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
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
	ResourceID         string          `json:"resourceId"`
	ResourceLabel      string          `json:"resourceLabel"`
	RelativePath       string          `json:"relativePath"`
	RecycleBin         bool            `json:"recycleBin"`
	ExpectedGeneration *int64          `json:"expectedGeneration"`
}

type shareResponse struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Path          string                 `json:"path"`
	ResourceID    string                 `json:"resourceId"`
	ResourceLabel string                 `json:"resourceLabel"`
	RelativePath  string                 `json:"relativePath"`
	Description   string                 `json:"description,omitempty"`
	Enabled       bool                   `json:"enabled"`
	Guest         bool                   `json:"guest"`
	RecycleBin    bool                   `json:"recycleBin"`
	Status        string                 `json:"status"`
	Protocols     []shareProtocolSetting `json:"protocols"`
	Access        []shareAccessSetting   `json:"access"`
}

type shareProtocolSetting struct {
	Protocol   string `json:"protocol"`
	Enabled    bool   `json:"enabled"`
	Hosts      string `json:"hosts,omitempty"`
	ReadOnly   bool   `json:"readOnly,omitempty"`
	QuotaBytes int64  `json:"quotaBytes,omitempty"`
}

type shareAccessSetting struct {
	PrincipalID string `json:"principalId"`
	Level       string `json:"level"`
}

func presentShare(share shares.ManagedShare) shareResponse {
	resourceID := share.Path
	resourceLabel := share.Path
	relativePath := "/"
	if resourceID == "" {
		resourceID = share.ID
	}
	protocols := make([]shareProtocolSetting, 0, len(share.Protocols))
	for _, protocol := range share.Protocols {
		setting := shareProtocolSetting{Protocol: protocol.Name, Enabled: true}
		if value, ok := protocol.Settings["hosts"].(string); ok {
			setting.Hosts = value
		}
		if value, ok := protocol.Settings["readOnly"].(bool); ok {
			setting.ReadOnly = value
		}
		if value, ok := protocol.Settings["quotaBytes"].(float64); ok {
			setting.QuotaBytes = int64(value)
		}
		protocols = append(protocols, setting)
	}
	access := make([]shareAccessSetting, 0, len(share.Access))
	for _, rule := range share.Access {
		access = append(access, shareAccessSetting{PrincipalID: rule.PrincipalID, Level: rule.Level})
	}
	status := "healthy"
	if !share.Enabled {
		status = "attention"
	}
	return shareResponse{
		ID: share.ID, Name: share.Name, Path: share.Path,
		ResourceID: resourceID, ResourceLabel: resourceLabel, RelativePath: relativePath,
		Description: share.Description, Enabled: share.Enabled, Guest: share.Guest,
		Status: status, Protocols: protocols, Access: access,
	}
}

func writeShare(w http.ResponseWriter, status int, share shares.ManagedShare) {
	writeJSON(w, status, presentShare(share))
}

func decodeManagedShare(r *http.Request) (shares.ManagedShare, *int64, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return shares.ManagedShare{}, nil, errors.New("invalid JSON")
	}
	return decodeManagedShareBytes(raw)
}

func decodeManagedShareBytes(raw []byte) (shares.ManagedShare, *int64, error) {
	var payload sharePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
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
	pathValue := payload.Path
	if pathValue == "" {
		pathValue = modernSharePath(payload.ResourceID, payload.RelativePath, payload.Name)
	}
	return shares.ManagedShare{ID: payload.ID, Name: payload.Name, Path: pathValue, Description: payload.Description, Enabled: payload.Enabled, Guest: payload.Guest, Protocols: protocols, Access: access}, payload.ExpectedGeneration, nil
}

func modernSharePath(resourceID, relativePath, name string) string {
	base := strings.TrimSpace(resourceID)
	base = strings.TrimPrefix(base, "/")
	base = strings.ReplaceAll(base, "..", "")
	if base == "" {
		base = "share-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	}
	base = filepath.Join("/srv/pools", base)
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" || relativePath == "/" {
		return base
	}
	return filepath.Join(base, path.Clean("/"+relativePath))
}

func decodeProtocols(raw json.RawMessage) ([]shares.Protocol, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("at least one share protocol is required")
	}
	var modern []struct {
		Protocol   string `json:"protocol"`
		Enabled    bool   `json:"enabled"`
		Hosts      string `json:"hosts"`
		ReadOnly   bool   `json:"readOnly"`
		QuotaBytes int64  `json:"quotaBytes"`
	}
	if err := json.Unmarshal(raw, &modern); err == nil && len(modern) > 0 && modern[0].Protocol != "" {
		result := make([]shares.Protocol, 0, len(modern))
		for _, item := range modern {
			if !item.Enabled {
				continue
			}
			settings := map[string]any{}
			if item.Hosts != "" {
				settings["hosts"] = item.Hosts
			}
			if item.ReadOnly {
				settings["readOnly"] = true
			}
			if item.QuotaBytes > 0 {
				settings["quotaBytes"] = item.QuotaBytes
			}
			result = append(result, shares.Protocol{Name: item.Protocol, Settings: settings})
		}
		if len(result) == 0 {
			return nil, errors.New("at least one share protocol must be enabled")
		}
		return result, nil
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
	responses := make([]shareResponse, 0, len(values))
	for _, value := range values {
		responses = append(responses, presentShare(value))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (s *apiServer) getManagedShare(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.store.ManagedShare(id)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeShare(w, http.StatusOK, share)
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
	idempotencyKey, err := requestIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if idempotencyKey != "" {
		if shareID, found := s.store.Meta(idempotencyMetaKey("share.create", idempotencyKey)); found {
			if existing, getErr := s.store.ManagedShare(shareID); getErr == nil {
				writeShare(w, http.StatusOK, existing)
				return
			}
		}
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
	prepared, err := s.prepareShareConfigs(candidate)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.store.CreateManagedShare(share)
	if err != nil {
		cleanupShareConfigs(prepared)
		writeJSON(w, statusForShareError(err), map[string]string{"error": err.Error()})
		return
	}
	if err := activateShareConfigs(prepared); err != nil {
		_ = s.store.DeleteManagedShare(created.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate Samba configuration: " + err.Error()})
		return
	}
	if err := s.reloadShareServices(r.Context(), candidate); err != nil {
		restoreShareConfigs(prepared)
		_ = s.store.DeleteManagedShare(created.ID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activate share services: " + err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "share.create", created.ID, map[string]any{"name": created.Name})
	if idempotencyKey != "" {
		_ = s.store.SetMeta(idempotencyMetaKey("share.create", idempotencyKey), created.ID)
	}
	s.advanceGeneration("share.create")
	writeShare(w, http.StatusCreated, created)
}

func (s *apiServer) updateManagedShare(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	var payload sharePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid JSON"})
		return
	}
	var share shares.ManagedShare
	var expectedGeneration *int64
	if len(payload.Protocols) == 0 || string(payload.Protocols) == "null" {
		share, err = s.store.ManagedShare(id)
		if err == nil && payload.Description != "" {
			share.Description = payload.Description
		}
		expectedGeneration = payload.ExpectedGeneration
	} else {
		share, expectedGeneration, err = decodeManagedShareBytes(raw)
	}
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if !s.expectedIdentityGeneration(w, expectedGeneration) {
		return
	}
	share.ID = id
	s.commitManagedShareUpdate(w, r, actor, share)
}

func (s *apiServer) commitManagedShareUpdate(w http.ResponseWriter, r *http.Request, actor string, share shares.ManagedShare) {
	existing, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	found := false
	var previous shares.ManagedShare
	for index := range existing {
		if existing[index].ID == share.ID {
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
	prepared, err := s.prepareShareConfigs(existing)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	updated, err := s.store.UpdateManagedShare(share)
	if err != nil {
		cleanupShareConfigs(prepared)
		writeJSON(w, statusForShareError(err), map[string]string{"error": err.Error()})
		return
	}
	if err := activateShareConfigs(prepared); err != nil {
		_, _ = s.store.UpdateManagedShare(previous)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate Samba configuration: " + err.Error()})
		return
	}
	if err := s.reloadShareServices(r.Context(), existing); err != nil {
		restoreShareConfigs(prepared)
		_, _ = s.store.UpdateManagedShare(previous)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activate share services: " + err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "share.update", updated.ID, map[string]any{"name": updated.Name})
	s.advanceGeneration("share.update")
	writeShare(w, http.StatusOK, updated)
}

func (s *apiServer) updateShareAccess(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		PrincipalID string `json:"principalId"`
		Level       string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	share, err := s.store.ManagedShare(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	updated := false
	for index := range share.Access {
		if share.Access[index].PrincipalID == input.PrincipalID {
			share.Access[index].Level = input.Level
			updated = true
			break
		}
	}
	if !updated {
		if _, err := s.store.Principal(input.PrincipalID); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "principal not found"})
			return
		}
		share.Access = append(share.Access, shares.AccessRule{PrincipalID: input.PrincipalID, Level: input.Level})
	}
	s.commitManagedShareUpdate(w, r, actor, share)
}

func (s *apiServer) updateShareProtocol(w http.ResponseWriter, r *http.Request, endpoint string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	parts := strings.Split(strings.TrimPrefix(endpoint, "/shares/"), "/protocols/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid share protocol path"})
		return
	}
	var input struct {
		Enabled    *bool   `json:"enabled"`
		Hosts      *string `json:"hosts"`
		ReadOnly   *bool   `json:"readOnly"`
		QuotaBytes *int64  `json:"quotaBytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	share, err := s.store.ManagedShare(parts[0])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	protocol := parts[1]
	index := -1
	for current := range share.Protocols {
		if share.Protocols[current].Name == protocol {
			index = current
			break
		}
	}
	if input.Enabled != nil && !*input.Enabled {
		if index >= 0 {
			share.Protocols = append(share.Protocols[:index], share.Protocols[index+1:]...)
		}
	} else {
		if index < 0 {
			share.Protocols = append(share.Protocols, shares.Protocol{Name: protocol, Settings: map[string]any{}})
			index = len(share.Protocols) - 1
		}
		settings := share.Protocols[index].Settings
		if settings == nil {
			settings = map[string]any{}
		}
		if input.Hosts != nil {
			settings["hosts"] = *input.Hosts
		}
		if input.ReadOnly != nil {
			settings["readOnly"] = *input.ReadOnly
		}
		if input.QuotaBytes != nil {
			settings["quotaBytes"] = *input.QuotaBytes
		}
		share.Protocols[index].Settings = settings
	}
	s.commitManagedShareUpdate(w, r, actor, share)
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
	prepared, err := s.prepareShareConfigs(filtered)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.DeleteManagedShare(id); err != nil {
		cleanupShareConfigs(prepared)
		writeJSON(w, statusForShareError(err), map[string]string{"error": err.Error()})
		return
	}
	if err := activateShareConfigs(prepared); err != nil {
		_, _ = s.store.CreateManagedShare(existingShareByID(existing, id))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate Samba configuration: " + err.Error()})
		return
	}
	if err := s.reloadShareServices(r.Context(), filtered); err != nil {
		restoreShareConfigs(prepared)
		_, _ = s.store.CreateManagedShare(existingShareByID(existing, id))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activate share services: " + err.Error()})
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

func statusForShareError(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	if strings.Contains(err.Error(), "already exists") {
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}
