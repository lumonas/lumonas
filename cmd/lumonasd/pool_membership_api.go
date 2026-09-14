package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

// planPoolMembership grows a mounted mergerfs pool: the plan seals which
// disks join, which of them need formatting, and the resulting branch list.
func (s *apiServer) planPoolMembership(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Pool               string   `json:"pool"`
		AddDiskIDs         []string `json:"addDiskIds"`
		Filesystem         string   `json:"filesystem"`
		ForceFormat        bool     `json:"forceFormat"`
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
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var pool *model.Pool
	for _, candidate := range s.discoverPools(ctx, disks) {
		if candidate.Name == input.Pool {
			pool = &candidate
			break
		}
	}
	if pool == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pool is not mounted: " + input.Pool})
		return
	}
	plan, err := storage.NewPoolMembershipPlan(newID("pool-grow"), *pool, disks, input.AddDiskIDs, input.Filesystem, input.ForceFormat, s.currentGeneration(), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.SavePoolMembershipPlan(plan); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("storage.pool.membership.planned", "warning", &model.ResourceRef{Type: "pool", ID: plan.PoolName}, map[string]any{"operationId": plan.OperationID, "planHash": plan.PlanHash, "add": len(plan.AddDiskIDs), "destroysData": len(plan.FormatDiskIDs) > 0})
	writeJSON(w, http.StatusCreated, plan)
}

// confirmPoolMembership executes the grow: format new members, unmount the
// pool, re-mount over the full branch list, extend the SnapRAID layout with
// fresh data slots, and queue a sync.
func (s *apiServer) confirmPoolMembership(w http.ResponseWriter, r *http.Request) {
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
	plan, err := s.store.PoolMembershipPlan(input.OperationID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pool membership plan not found"})
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
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := storage.ValidatePoolMembershipPlan(plan, s.discoverPools(ctx, disks), disks, time.Now().UTC(), s.currentGeneration()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pool membership revalidation failed: " + err.Error()})
		return
	}
	byID := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	// Step 1: format and mount each new member at its branch path.
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
		formatRequest := privileged.Request{Operation: string(single.Action), OperationID: single.OperationID, CorrelationID: requestCorrelationID(r), PlanHash: single.PlanHash, TargetDiskID: single.Target.DiskID, ExpectedIdentity: expectedIdentityMap(single.Target), ExpectedState: expectedStateMap(single.ExpectedState), RequestedState: single.RequestedState, ExpiresAt: single.ExpiresAt, Confirmed: true}
		if execErr := s.brokerExecute(r.Context(), formatRequest); execErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "formatting " + id + " failed: " + execErr.Error()})
			return
		}
	}
	// Step 2: unmount the pool (branches stay mounted).
	if err := s.brokerExecute(r.Context(), privileged.Request{Operation: "pool.unmount", OperationID: newID("pool-unmount"), PlanHash: "pool-unmount-" + plan.OperationID, RequestedState: map[string]any{"mountPath": plan.MountPath}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Confirmed: true}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pool unmount failed: " + err.Error()})
		return
	}
	// Step 3: re-mount over the full branch list with fresh identities.
	disks, err = s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity re-scan unavailable"})
		return
	}
	branchToDisk := make(map[string]model.Disk, len(disks))
	byID = make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		branchToDisk[storage.DiskBranchPath(disk.ID)] = disk
		byID[disk.ID] = disk
	}
	members := make([]model.Disk, 0, len(plan.NewBranches))
	for _, branch := range plan.NewBranches {
		disk, ok := branchToDisk[branch]
		if !ok {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "branch " + branch + " has no discovered disk identity"})
			return
		}
		members = append(members, disk)
	}
	poolPlan, err := storage.NewPoolPlan(plan.OperationID, plan.PoolName, plan.MountPath, members, s.currentGeneration(), time.Now().UTC())
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
	mountRequest := privileged.Request{Operation: "pool.mount", OperationID: poolPlan.OperationID, PlanHash: poolPlan.PlanHash, ExpectedDisks: expected, RequestedState: map[string]any{"mountPath": poolPlan.MountPath, "branches": branches, "policy": poolPlan.Policy}, ExpiresAt: plan.ExpiresAt, Confirmed: true}
	if err := s.brokerExecute(r.Context(), mountRequest); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pool mount failed: " + err.Error()})
		return
	}
	s.advanceGeneration("storage.pool.membership")
	s.persistMountState("storage.pool.membership")
	// Step 4: extend the SnapRAID layout when the config is managed and has
	// parity. Slot disk ids are branch-sanitized; match identities through
	// branch paths.
	protectionUpdated := false
	if parity, slots, parseErr := s.managedSnapraidLayout(); parseErr == nil && parity != "" && !slotDiskIDEqualsParity(slots, parity) {
		updated, extendErr := storage.ExtendSnapraidSlots(slots, plan.AddDiskIDs)
		if extendErr == nil {
			slotsPayload := make([]any, 0, len(updated))
			expectedDisks := make([]privileged.ExpectedDisk, 0, len(updated)+1)
			for _, slot := range updated {
				slotsPayload = append(slotsPayload, map[string]any{"name": slot.Name, "diskId": slot.DiskID})
				if disk, ok := branchToDisk[storage.DiskBranchPath(slot.DiskID)]; ok {
					expectedDisks = append(expectedDisks, privileged.ExpectedDisk{ID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, GPTDiskGUID: disk.GPTDiskGUID, PartitionUUID: disk.PartitionUUID, FilesystemUUID: disk.FilesystemUUID})
				}
			}
			if disk, ok := branchToDisk[storage.DiskBranchPath(parity)]; ok {
				expectedDisks = append(expectedDisks, privileged.ExpectedDisk{ID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, GPTDiskGUID: disk.GPTDiskGUID, PartitionUUID: disk.PartitionUUID, FilesystemUUID: disk.FilesystemUUID})
			}
			configRequest := privileged.Request{Operation: "snapraid.config.apply", OperationID: newID("snapraid-config"), PlanHash: plan.PlanHash, ExpectedDisks: expectedDisks, RequestedState: map[string]any{"configPath": s.snapraidConfigPath(), "dataSlots": slotsPayload}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Confirmed: true}
			if configErr := s.brokerExecute(r.Context(), configRequest); configErr == nil {
				protectionUpdated = true
			}
		}
	}
	// Step 5: queue a sync so parity covers the new data.
	syncJob := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Type: "snapraid.sync", Title: "snapraid sync", ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
	if err := s.store.SaveJob(syncJob); err == nil {
		s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: syncJob.ID}, map[string]any{"job": syncJob})
		go s.runProtectionJob(syncJob)
	}
	s.publish("storage.pool.membership.completed", "info", &model.ResourceRef{Type: "pool", ID: plan.PoolName}, map[string]any{"operationId": plan.OperationID, "added": len(plan.AddDiskIDs), "protectionUpdated": protectionUpdated})
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "mountPath": plan.MountPath, "added": plan.AddDiskIDs, "members": len(plan.NewBranches), "protectionUpdated": protectionUpdated})
}

// managedSnapraidLayout parses the active SnapRAID configuration; unknown
// configurations return an error so callers can skip protection updates.
func (s *apiServer) managedSnapraidLayout() (string, []storage.DataSlot, error) {
	config, err := os.ReadFile(s.snapraidConfigPath())
	if err != nil {
		return "", nil, err
	}
	return storage.ParseSnapraidDataMapping(string(config))
}

func slotDiskIDEqualsParity(slots []storage.DataSlot, parity string) bool {
	for _, slot := range slots {
		if storage.DiskBranchPath(slot.DiskID) == storage.DiskBranchPath(parity) {
			return true
		}
	}
	return false
}

// discoverPools wraps pool discovery with a test seam; production consults
// the system mount table.
func (s *apiServer) discoverPools(ctx context.Context, disks []model.Disk) []model.Pool {
	if s.discoverPoolsFunc != nil {
		return s.discoverPoolsFunc(disks)
	}
	return storage.DiscoverPools(ctx, disks, nil)
}
