package main

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

func (s *apiServer) planStoragePool(w http.ResponseWriter, r *http.Request) {
	idempotencyKey, err := requestIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if idempotencyKey != "" {
		if operationID, found := s.store.Meta(idempotencyMetaKey("storage.pool.plan", idempotencyKey)); found {
			if existing, loadErr := s.store.PoolPlan(operationID); loadErr == nil {
				writeJSON(w, http.StatusOK, existing)
				return
			}
		}
	}
	var input struct {
		Name               string   `json:"name"`
		DiskIDs            []string `json:"diskIds"`
		ExpectedGeneration *int64   `json:"expectedGeneration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.ExpectedGeneration != nil && *input.ExpectedGeneration != s.currentGeneration() {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "configuration generation changed", "currentGeneration": s.currentGeneration()})
		return
	}
	if len(input.DiskIDs) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "diskIds must contain at least one stable disk identity"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	byID := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	selected := make([]model.Disk, 0, len(input.DiskIDs))
	for _, id := range input.DiskIDs {
		disk, ok := byID[id]
		if !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "diskId is not a currently discovered stable identity: " + id})
			return
		}
		selected = append(selected, disk)
	}
	operationID := newID("pool")
	plan, err := storage.NewPoolPlan(operationID, input.Name, "/srv/pools/"+input.Name, selected, s.currentGeneration(), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.SavePoolPlan(plan); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if idempotencyKey != "" {
		_ = s.store.SetMeta(idempotencyMetaKey("storage.pool.plan", idempotencyKey), plan.OperationID)
	}
	s.publish("storage.pool.planned", "warning", &model.ResourceRef{Type: "pool", ID: plan.Name}, map[string]any{"operationId": plan.OperationID, "planHash": plan.PlanHash, "diskCount": len(plan.Members)})
	writeJSON(w, http.StatusCreated, plan)
}

func (s *apiServer) confirmStoragePool(w http.ResponseWriter, r *http.Request, operationID string) {
	plan, err := s.store.PoolPlan(operationID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pool plan not found"})
		return
	}
	var input struct {
		PlanHash              string `json:"planHash"`
		Reauthenticated       bool   `json:"reauthenticated"`
		StorageSafetyUnlocked bool   `json:"storageSafetyUnlocked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.PlanHash == "" || input.PlanHash != plan.PlanHash {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plan hash mismatch"})
		return
	}
	if !input.Reauthenticated || !input.StorageSafetyUnlocked || !s.safetyUnlocked() {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication and the storage safety unlock are required"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity revalidation unavailable"})
		return
	}
	if err := storage.ValidatePoolPlan(plan, disks, time.Now().UTC(), s.currentGeneration()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pool plan revalidation failed: " + err.Error()})
		return
	}
	expected := make([]privileged.ExpectedDisk, 0, len(plan.Members))
	branches := make([]any, 0, len(plan.Members))
	for _, member := range plan.Members {
		expected = append(expected, privileged.ExpectedDisk{ID: member.DiskID, WWN: member.WWN, Serial: member.Serial, Model: member.Model, SizeBytes: member.SizeBytes, FilesystemUUID: member.FilesystemUUID})
		branches = append(branches, member.BranchPath)
	}
	request := privileged.Request{Operation: "pool.mount", OperationID: plan.OperationID, PlanHash: plan.PlanHash, ExpectedDisks: expected, RequestedState: map[string]any{"mountPath": plan.MountPath, "branches": branches, "policy": plan.Policy}, ExpiresAt: plan.ExpiresAt, Confirmed: true}
	result, err := (privileged.Client{Socket: envOr("MYNAS_PRIVD_SOCKET", "/run/mynas/privd.sock")}).Execute(r.Context(), request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.advanceGeneration("storage.pool.mount")
	s.publish("storage.pool.mounted", "info", &model.ResourceRef{Type: "pool", ID: plan.Name}, map[string]any{"operationId": plan.OperationID, "mountPath": plan.MountPath})
	writeJSON(w, http.StatusAccepted, result)
}

func poolOperationID(endpoint string) string {
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) == 4 && parts[0] == "storage" && parts[1] == "pools" && parts[3] == "confirm" {
		return path.Base(parts[2])
	}
	return ""
}
