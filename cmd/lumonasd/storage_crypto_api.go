package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

func (s *apiServer) unlockEncryptedDisk(w http.ResponseWriter, r *http.Request, diskID string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(input.Passphrase) < 1 || len(input.Passphrase) > 256 || strings.ContainsAny(input.Passphrase, "\r\n\x00") {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "passphrase must be 1–256 characters without line breaks"})
		return
	}
	if !s.safetyUnlocked() {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "unlock the storage safety window before opening this encrypted disk"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	var disk *model.Disk
	for index := range disks {
		if disks[index].ID == diskID {
			disk = &disks[index]
			break
		}
	}
	if disk == nil || !model.HasStableDiskIdentity(disk.ID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stable disk identity not found"})
		return
	}
	if disk.Filesystem != "crypto_LUKS" || disk.Mounted || disk.PoolID != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "disk is not an available locked LUKS volume"})
		return
	}
	operationID := newID("crypto-unlock")
	expires := time.Now().UTC().Add(2 * time.Minute)
	identity := map[string]string{
		"id": disk.ID, "wwn": disk.WWN, "serial": disk.Serial, "model": disk.Model,
		"gptDiskGuid": disk.GPTDiskGUID, "partitionUuid": disk.PartitionUUID,
		"filesystemUuid": disk.FilesystemUUID, "sizeBytes": strconv.FormatUint(disk.SizeBytes, 10),
	}
	result, err := s.executePrivileged(r.Context(), privileged.Request{
		Operation: "filesystem.crypto.unlock", OperationID: operationID, PlanHash: operationID,
		CorrelationID: requestCorrelationID(r), TargetDiskID: disk.ID, ExpectedIdentity: identity,
		RequestedState: map[string]any{"mountPath": storage.DiskBranchPath(disk.ID), "encryptionPassphrase": input.Passphrase},
		ExpiresAt:      expires, Confirmed: true,
	})
	input.Passphrase = ""
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "privileged unlock request failed"})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.advanceGeneration("storage.crypto.unlock")
	s.recordRequestAudit(r, actor, "storage.crypto.unlock", disk.ID, map[string]any{"operationId": operationID})
	s.publishActor(actor, "storage.crypto.unlocked", "warning", &model.ResourceRef{Type: "disk", ID: disk.ID}, map[string]any{"operationId": operationID})
	s.persistMountState("storage.crypto.unlock")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mountPath": storage.DiskBranchPath(disk.ID)})
}
