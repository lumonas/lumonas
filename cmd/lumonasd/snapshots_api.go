package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
	"github.com/lumonas/lumonas/internal/store"
)

// storageSnapshots lists persisted snapshot records, optionally scoped to a
// source with ?source=.
func (s *apiServer) storageSnapshots(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		limit = value
	}
	records, err := s.store.StorageSnapshots(r.URL.Query().Get("source"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, records)
}

// createStorageSnapshot takes one read-only snapshot through the privileged
// broker and persists the result. Creation is cheap and reversible, so it
// needs management identity but not the destructive-storage unlock.
func (s *apiServer) createStorageSnapshot(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Kind              string `json:"kind"`
		Source            string `json:"source"`
		Label             string `json:"label"`
		RetentionLockDays int    `json:"retentionLockDays"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	kind := storage.SnapshotKind(strings.ToLower(input.Kind))
	if err := storage.ValidateSnapshotSource(kind, input.Source); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := storage.ValidateSnapshotLabel(input.Label); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if input.RetentionLockDays < 0 || input.RetentionLockDays > 3650 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot retention lock must be between 0 and 3650 days"})
		return
	}
	operationID := newID("snap")
	request := privileged.Request{
		Operation:   "snapshot.create",
		OperationID: operationID,
		PlanHash:    operationID,
		RequestedState: map[string]any{
			"kind":   string(kind),
			"source": input.Source,
			"label":  input.Label,
		},
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
		Confirmed: true,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	result, err := s.executePrivileged(ctx, request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	record := snapshotRecordFromResponse(result, kind, input.Source, input.Label)
	if input.RetentionLockDays > 0 {
		protectedUntil := time.Now().UTC().Add(time.Duration(input.RetentionLockDays) * 24 * time.Hour)
		record.ProtectedUntil = &protectedUntil
	}
	saved, err := s.store.SaveStorageSnapshot(record)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	record = saved
	s.recordRequestAudit(r, actor, "storage.snapshot.create", record.ID, map[string]any{"kind": string(kind), "source": input.Source, "name": record.Name})
	s.publish("storage.snapshot.created", "info", &model.ResourceRef{Type: "snapshot", ID: record.ID}, map[string]any{"snapshotId": record.ID, "source": input.Source, "name": record.Name})
	writeJSON(w, http.StatusCreated, record)
}

// deleteStorageSnapshot destroys a snapshot through the privileged broker and
// drops the persisted row only after the broker confirms.
func (s *apiServer) deleteStorageSnapshot(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Reauthenticated       bool `json:"reauthenticated"`
		StorageSafetyUnlocked bool `json:"storageSafetyUnlocked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.Reauthenticated || !input.StorageSafetyUnlocked || !s.safetyUnlocked() {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication and the storage safety unlock are required"})
		return
	}
	record, found, err := s.store.StorageSnapshot(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
		return
	}
	if record.ProtectedUntil != nil && record.ProtectedUntil.After(time.Now().UTC()) {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "snapshot is retention-locked until " + record.ProtectedUntil.UTC().Format(time.RFC3339)})
		return
	}
	operationID := newID("snap")
	request := privileged.Request{
		Operation:   "snapshot.delete",
		OperationID: operationID,
		PlanHash:    operationID,
		RequestedState: map[string]any{
			"kind":   record.Kind,
			"source": record.Source,
			"name":   record.Name,
		},
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
		Confirmed: true,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	result, err := s.executePrivileged(ctx, request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	if err := s.store.DeleteStorageSnapshot(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "storage.snapshot.delete", id, map[string]any{"kind": record.Kind, "source": record.Source, "name": record.Name})
	s.publish("storage.snapshot.deleted", "info", &model.ResourceRef{Type: "snapshot", ID: id}, map[string]any{"snapshotId": id, "source": record.Source, "name": record.Name})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// snapshotRecordFromResponse prefers the broker's authoritative snapshot
// metadata; the brokerExec test seam returns no payload, so the expected
// record is synthesized from the request in that case. It does not persist.
func snapshotRecordFromResponse(result privileged.Response, kind storage.SnapshotKind, source, label string) store.StorageSnapshotRecord {
	if data, ok := result.Data.(storage.Snapshot); ok {
		return store.StorageSnapshotRecord{Kind: string(data.Kind), Source: data.Source, Name: data.Name, Label: label, CreatedAt: data.CreatedAt}
	}
	if data, ok := result.Data.(map[string]any); ok {
		name, _ := data["name"].(string)
		kindValue, _ := data["kind"].(string)
		sourceValue, _ := data["source"].(string)
		if name != "" {
			recordKind := kind
			if kindValue != "" {
				recordKind = storage.SnapshotKind(kindValue)
			}
			if sourceValue == "" {
				sourceValue = source
			}
			return store.StorageSnapshotRecord{Kind: string(recordKind), Source: sourceValue, Name: name, Label: label, CreatedAt: time.Now().UTC()}
		}
	}
	return store.StorageSnapshotRecord{Kind: string(kind), Source: source, Name: storage.SnapshotName(label, time.Now().UTC()), Label: label, CreatedAt: time.Now().UTC()}
}
