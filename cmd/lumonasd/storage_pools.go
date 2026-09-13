package main

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

func (s *apiServer) planStoragePoolSetup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name               string   `json:"name"`
		DataDiskIDs        []string `json:"dataDiskIds"`
		ParityDiskID       string   `json:"parityDiskId"`
		Filesystem         string   `json:"filesystem"`
		FormatDisks        bool     `json:"formatDisks"`
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
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	plan, err := storage.NewPoolSetupPlan(newID("pool-setup"), input.Name, disks, input.DataDiskIDs, strings.TrimSpace(input.ParityDiskID), input.Filesystem, input.FormatDisks, s.currentGeneration(), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.SavePoolSetupPlan(plan); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("storage.pool.setup.planned", "warning", &model.ResourceRef{Type: "pool", ID: plan.Name}, map[string]any{"operationId": plan.OperationID, "planHash": plan.PlanHash, "destroysData": plan.DestroysData, "steps": len(plan.Steps)})
	writeJSON(w, http.StatusCreated, plan)
}

func (s *apiServer) confirmStoragePoolSetup(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		OperationID           string `json:"operationId"`
		PlanHash              string `json:"planHash"`
		Reauthenticated       bool   `json:"reauthenticated"`
		StorageSafetyUnlocked bool   `json:"storageSafetyUnlocked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.OperationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "operationId is required"})
		return
	}
	plan, err := s.store.PoolSetupPlan(input.OperationID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pool setup plan not found"})
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
	if err := storage.ValidatePoolSetupPlan(plan, disks, time.Now().UTC(), s.currentGeneration()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pool setup revalidation failed: " + err.Error()})
		return
	}
	byID := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	broker := func(request privileged.Request) error { return s.brokerExecute(r.Context(), request) }
	formatted := make([]string, 0, len(plan.FormatDiskIDs))
	for _, id := range plan.FormatDiskIDs {
		disk := byID[id]
		single, planErr := storage.NewPlan(newID("disk"), storage.ActionCreate, disk, s.currentGeneration(), time.Now().UTC())
		if planErr != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "format plan for " + id + " failed: " + planErr.Error()})
			return
		}
		single.RequestedState = map[string]any{"filesystem": plan.Filesystem, "mountPath": storage.DiskBranchPath(id)}
		single.Status = "confirmed"
		single.PlanHash = storage.Hash(single)
		request := privileged.Request{Operation: string(single.Action), OperationID: single.OperationID, CorrelationID: requestCorrelationID(r), PlanHash: single.PlanHash, TargetDiskID: single.Target.DiskID, ExpectedIdentity: expectedIdentityMap(single.Target), ExpectedState: expectedStateMap(single.ExpectedState), RequestedState: single.RequestedState, ExpiresAt: single.ExpiresAt, Confirmed: true}
		if execErr := broker(request); execErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "formatting " + id + " failed: " + execErr.Error()})
			return
		}
		formatted = append(formatted, id)
	}
	// Re-scan after formatting: filesystem UUIDs changed and the pool plan
	// must be built from the fresh identities.
	disks, err = s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity re-scan unavailable"})
		return
	}
	byID = make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	for _, id := range plan.MountDiskIDs {
		disk, ok := byID[id]
		if !ok {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "mount disk " + id + " disappeared during setup"})
			return
		}
		if disk.Filesystem != "ext4" && disk.Filesystem != "xfs" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "mount disk " + id + " has no supported filesystem"})
			return
		}
		single, planErr := storage.NewPlan(newID("disk-mount"), storage.ActionMount, disk, s.currentGeneration(), time.Now().UTC())
		if planErr != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "mount plan for " + id + " failed: " + planErr.Error()})
			return
		}
		single.RequestedState = map[string]any{"filesystem": disk.Filesystem, "mountPath": storage.DiskBranchPath(id)}
		single.Status = "confirmed"
		single.PlanHash = storage.Hash(single)
		request := privileged.Request{Operation: string(single.Action), OperationID: single.OperationID, CorrelationID: requestCorrelationID(r), PlanHash: single.PlanHash, TargetDiskID: single.Target.DiskID, ExpectedIdentity: expectedIdentityMap(single.Target), ExpectedState: expectedStateMap(single.ExpectedState), RequestedState: single.RequestedState, ExpiresAt: single.ExpiresAt, Confirmed: true}
		if execErr := broker(request); execErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "mounting " + id + " failed: " + execErr.Error()})
			return
		}
	}
	if len(plan.MountDiskIDs) > 0 {
		disks, err = s.diskFunc()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity re-scan after mounting unavailable"})
			return
		}
		byID = make(map[string]model.Disk, len(disks))
		for _, disk := range disks {
			byID[disk.ID] = disk
		}
	}
	members := make([]model.Disk, 0, len(plan.DataDiskIDs))
	for _, id := range plan.DataDiskIDs {
		disk, ok := byID[id]
		if !ok {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "pool disk " + id + " disappeared during setup"})
			return
		}
		members = append(members, disk)
	}
	poolPlan, err := storage.NewPoolPlan(plan.OperationID, plan.Name, plan.MountPath, members, s.currentGeneration(), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pool plan build failed: " + err.Error()})
		return
	}
	poolPlan.Status = "confirmed"
	poolPlan.PlanHash = storage.HashPoolPlan(poolPlan)
	expected := make([]privileged.ExpectedDisk, 0, len(poolPlan.Members))
	branches := make([]any, 0, len(poolPlan.Members))
	for _, member := range poolPlan.Members {
		expected = append(expected, privileged.ExpectedDisk{ID: member.DiskID, WWN: member.WWN, Serial: member.Serial, Model: member.Model, SizeBytes: member.SizeBytes, GPTDiskGUID: member.GPTDiskGUID, PartitionUUID: member.PartitionUUID, FilesystemUUID: member.FilesystemUUID})
		branches = append(branches, member.BranchPath)
	}
	request := privileged.Request{Operation: "pool.mount", OperationID: poolPlan.OperationID, PlanHash: poolPlan.PlanHash, ExpectedDisks: expected, RequestedState: map[string]any{"mountPath": poolPlan.MountPath, "branches": branches, "policy": poolPlan.Policy}, ExpiresAt: plan.ExpiresAt, Confirmed: true}
	if err := broker(request); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	s.advanceGeneration("storage.pool.setup")
	s.publishActor(actor, "storage.pool.setup.completed", "info", &model.ResourceRef{Type: "pool", ID: plan.Name}, map[string]any{"operationId": plan.OperationID, "formatted": len(formatted), "members": len(plan.DataDiskIDs)})
	s.persistMountState("storage.pool.setup")
	protectionConfigured := false
	if plan.ParityDiskID != "" {
		protectionConfigured = s.applySnapraidConfiguration(plan.ParityDiskID, plan.DataDiskIDs)
		if protectionConfigured {
			job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Actor: actor, Type: "snapraid.sync", Title: "snapraid sync", ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
			if err := s.store.SaveJob(job); err == nil {
				s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
				go s.runProtectionJob(job)
			}
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "mountPath": plan.MountPath, "formatted": formatted, "protectionConfigured": protectionConfigured})
}

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
		expected = append(expected, privileged.ExpectedDisk{ID: member.DiskID, WWN: member.WWN, Serial: member.Serial, Model: member.Model, SizeBytes: member.SizeBytes, GPTDiskGUID: member.GPTDiskGUID, PartitionUUID: member.PartitionUUID, FilesystemUUID: member.FilesystemUUID})
		branches = append(branches, member.BranchPath)
	}
	request := privileged.Request{Operation: "pool.mount", OperationID: plan.OperationID, PlanHash: plan.PlanHash, ExpectedDisks: expected, RequestedState: map[string]any{"mountPath": plan.MountPath, "branches": branches, "policy": plan.Policy}, ExpiresAt: plan.ExpiresAt, Confirmed: true}
	result, err := s.executePrivileged(r.Context(), request)
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
	s.persistMountState("storage.pool.mount")
	writeJSON(w, http.StatusAccepted, result)
}

func (s *apiServer) planStoragePoolUnmount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name               string `json:"name"`
		ExpectedGeneration *int64 `json:"expectedGeneration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.ExpectedGeneration != nil && *input.ExpectedGeneration != s.currentGeneration() {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "configuration generation changed", "currentGeneration": s.currentGeneration()})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	for _, pool := range storage.DiscoverPools(ctx, disks, nil) {
		if pool.Name != input.Name {
			continue
		}
		plan, err := storage.NewPoolUnmountPlan(newID("pool-unmount"), pool, s.currentGeneration(), time.Now().UTC())
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		if err := s.store.SavePoolUnmountPlan(plan); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.publish("storage.pool.unmount.planned", "warning", &model.ResourceRef{Type: "pool", ID: pool.ID}, map[string]any{"operationId": plan.OperationID, "planHash": plan.PlanHash})
		writeJSON(w, http.StatusCreated, plan)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "pool is not mounted"})
}

func (s *apiServer) confirmStoragePoolUnmount(w http.ResponseWriter, r *http.Request, operationID string) {
	plan, err := s.store.PoolUnmountPlan(operationID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pool unmount plan not found"})
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
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	pools := storage.DiscoverPools(ctx, disks, nil)
	if err := storage.ValidatePoolUnmountPlan(plan, pools, time.Now().UTC(), s.currentGeneration()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pool unmount plan revalidation failed: " + err.Error()})
		return
	}
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "pool.unmount", OperationID: plan.OperationID, PlanHash: plan.PlanHash, RequestedState: map[string]any{"mountPath": plan.MountPath}, ExpiresAt: plan.ExpiresAt, Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.advanceGeneration("storage.pool.unmount")
	s.publish("storage.pool.unmounted", "warning", &model.ResourceRef{Type: "pool", ID: plan.PoolID}, map[string]any{"operationId": plan.OperationID, "mountPath": plan.MountPath})
	s.persistMountState("storage.pool.unmount")
	writeJSON(w, http.StatusAccepted, result)
}

func expectedIdentityMap(target storage.TargetIdentity) map[string]string {
	return map[string]string{"id": target.DiskID, "wwn": target.WWN, "serial": target.Serial, "model": target.Model, "gptDiskGuid": target.GPTDiskGUID, "partitionUuid": target.PartitionUUID, "filesystemUuid": target.FilesystemUUID, "sizeBytes": strconv.FormatUint(target.SizeBytes, 10)}
}

func expectedStateMap(state storage.ExpectedState) map[string]string {
	return map[string]string{"currentPath": state.CurrentPath, "mounted": strconv.FormatBool(state.Mounted), "role": state.Role, "poolId": state.PoolID}
}

func poolOperationID(endpoint string) string {
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) == 4 && parts[0] == "storage" && parts[1] == "pools" && parts[3] == "confirm" {
		return path.Base(parts[2])
	}
	return ""
}

func poolUnmountOperationID(endpoint string) string {
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) == 5 && parts[0] == "storage" && parts[1] == "pools" && parts[2] == "unmount" && parts[4] == "confirm" {
		return path.Base(parts[3])
	}
	return ""
}
