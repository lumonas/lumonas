package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

// planDiskReplacement builds the reviewed recovery sequence for a failed data
// disk: the replacement takes over the retired disk's SnapRAID slot name so
// `snapraid fix` can restore its content from parity.
func (s *apiServer) planDiskReplacement(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RetiredDiskID     string `json:"retiredDiskId"`
		ReplacementDiskID string `json:"replacementDiskId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	config, err := os.ReadFile(s.snapraidConfigPath())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "the current SnapRAID configuration could not be read; parity recovery needs it"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	plan, err := storage.NewReplacementPlan(newID("disk-replace"), strings.TrimSpace(input.RetiredDiskID), strings.TrimSpace(input.ReplacementDiskID), string(config), disks, s.currentGeneration(), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.SaveReplacementPlan(plan); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("storage.disk.replacement.planned", "warning", &model.ResourceRef{Type: "disk", ID: plan.ReplacementDiskID}, map[string]any{"operationId": plan.OperationID, "planHash": plan.PlanHash, "retired": plan.RetiredDiskID, "slot": plan.RetiredDataName})
	writeJSON(w, http.StatusCreated, plan)
}

func (s *apiServer) confirmDiskReplacement(w http.ResponseWriter, r *http.Request) {
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
	plan, err := s.store.ReplacementPlan(input.OperationID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replacement plan not found"})
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
	if err := storage.ValidateReplacementPlan(plan, disks, time.Now().UTC(), s.currentGeneration()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "replacement revalidation failed: " + err.Error()})
		return
	}
	var replacement model.Disk
	found := false
	for _, disk := range disks {
		if disk.ID == plan.ReplacementDiskID {
			replacement = disk
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "replacement disk is no longer present"})
		return
	}
	// Step 1: format and mount the replacement at its canonical branch path.
	single, planErr := storage.NewPlan(newID("disk"), storage.ActionCreate, replacement, s.currentGeneration(), time.Now().UTC())
	if planErr != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": planErr.Error()})
		return
	}
	single.RequestedState = map[string]any{"filesystem": "ext4", "mountPath": storage.DiskBranchPath(plan.ReplacementDiskID)}
	single.Status = "confirmed"
	single.PlanHash = storage.Hash(single)
	formatRequest := privileged.Request{Operation: string(single.Action), OperationID: single.OperationID, CorrelationID: requestCorrelationID(r), PlanHash: single.PlanHash, TargetDiskID: single.Target.DiskID, ExpectedIdentity: expectedIdentityMap(single.Target), ExpectedState: expectedStateMap(single.ExpectedState), RequestedState: single.RequestedState, ExpiresAt: single.ExpiresAt, Confirmed: true}
	if err := s.brokerExecute(r.Context(), formatRequest); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "replacement formatting failed: " + err.Error()})
		return
	}
	// Step 2: re-point the retired SnapRAID slot at the replacement. The
	// broker revalidates parity + every slot disk identity, so re-scan first.
	freshDisks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity re-scan unavailable"})
		return
	}
	byID := make(map[string]model.Disk, len(freshDisks))
	for _, disk := range freshDisks {
		byID[disk.ID] = disk
	}
	slots := make([]any, 0, len(plan.Slots))
	expected := make([]privileged.ExpectedDisk, 0, len(plan.Slots)+1)
	for _, slot := range plan.Slots {
		slots = append(slots, map[string]any{"name": slot.Name, "diskId": slot.DiskID})
		if disk, ok := byID[slot.DiskID]; ok {
			expected = append(expected, privileged.ExpectedDisk{ID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, GPTDiskGUID: disk.GPTDiskGUID, PartitionUUID: disk.PartitionUUID, FilesystemUUID: disk.FilesystemUUID})
		}
	}
	if plan.ParityDiskID != "" {
		if disk, ok := byID[plan.ParityDiskID]; ok {
			expected = append(expected, privileged.ExpectedDisk{ID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, GPTDiskGUID: disk.GPTDiskGUID, PartitionUUID: disk.PartitionUUID, FilesystemUUID: disk.FilesystemUUID})
		}
	}
	if err := s.brokerExecute(r.Context(), privileged.Request{Operation: "snapraid.config.apply", OperationID: newID("snapraid-config"), PlanHash: plan.PlanHash, ExpectedDisks: expected, RequestedState: map[string]any{"configPath": s.snapraidConfigPath(), "dataSlots": slots}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Confirmed: true}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SnapRAID reconfiguration failed: " + err.Error()})
		return
	}
	s.advanceGeneration("storage.disk.replacement")
	// Step 3: recover the retired slot's content from parity, then re-sync.
	fixJob := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), OperationID: plan.OperationID, PlanHash: plan.PlanHash, Actor: actor, Type: "snapraid.fix", Title: "snapraid fix " + plan.RetiredDataName, ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC(), Generation: plan.ConfigGeneration}
	s.setFixStage(fixJob.ID, plan.RetiredDataName)
	if err := s.store.SaveJob(fixJob); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	go s.runProtectionJob(fixJob)
	s.publishActor(actor, "storage.disk.replacement.started", "warning", &model.ResourceRef{Type: "disk", ID: plan.ReplacementDiskID}, map[string]any{"operationId": plan.OperationID, "jobId": fixJob.ID, "slot": plan.RetiredDataName})
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": fixJob.ID, "slot": plan.RetiredDataName, "next": "parity recovery will be followed by an automatic sync"})
}
