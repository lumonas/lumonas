package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/updates"
)

func (s *apiServer) updatePreflight(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	checks := make([]map[string]any, 0, 5)
	add := func(id, label string, passed bool, detail string, required bool) {
		checks = append(checks, map[string]any{"id": id, "label": label, "passed": passed, "detail": detail, "required": required})
	}
	publicKey, keyErr := updates.ParsePublicKey(osUpdatePublicKey())
	imagePath, manifest, state, imageErr := s.updateManager().StagedSlotImage()
	if keyErr == nil {
		imagePath, manifest, state, imageErr = s.updateManager().StagedSlotImageVerified(publicKey)
	} else {
		imageErr = keyErr
	}
	add("image-signature", "Staged image signature and digest", imageErr == nil, firstError(imageErr, "staged image verified against signed manifest"), true)
	mapping, mapErr := parseSlotDeviceMapping(os.Getenv("LUMONAS_SLOT_DEVICES"))
	rollbackReady := mapErr == nil && state.ActiveSlot != "" && mapping[state.ActiveSlot].Device != ""
	add("rollback-slot", "Known-good rollback slot", rollbackReady, firstError(mapErr, "active slot mapping is available"), true)
	key := s.recoveryKeyString()
	bundlePath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	bundle, bundleErr := os.ReadFile(bundlePath)
	var bundleGeneration int64
	if bundleErr == nil && key != "" {
		if verified, verifyErr := recovery.Verify(bundle, []byte(key)); verifyErr == nil {
			bundleGeneration = verified.Generation
		} else {
			bundleErr = verifyErr
		}
	} else if key == "" {
		bundleErr = errors.New("recovery key is not configured")
	}
	add("recovery-bundle", "Current verified recovery bundle", bundleErr == nil, firstError(bundleErr, "bundle signature and checksum verified"), true)
	currentGeneration := s.store.CurrentGeneration()
	canary := false
	if bundleErr == nil {
		for _, drill := range func() []model.RestoreDrill { values, _ := s.store.RestoreDrills(100); return values }() {
			if drill.State == "successful" && drill.ServicesHealthy && drill.Generation == bundleGeneration && drill.Generation == currentGeneration {
				canary = true
				break
			}
		}
	}
	add("recovery-canary", "Current-generation recovery canary", canary, map[bool]string{true: "latest configuration generation passed isolated service rehearsal", false: "run a successful restore drill for the latest verified generation"}[canary], true)
	updateRoot := envOr("LUMONAS_UPDATE_ROOT", "/var/lib/lumonas/updates")
	var filesystem syscall.Statfs_t
	spaceErr := syscall.Statfs(updateRoot, &filesystem)
	freeBytes := uint64(0)
	if spaceErr == nil {
		freeBytes = filesystem.Bavail * uint64(filesystem.Bsize)
	}
	add("staging-space", "Update storage available", spaceErr == nil && freeBytes > 0, fmt.Sprintf("%d bytes free in %s", freeBytes, updateRoot), true)
	ready := true
	for _, check := range checks {
		if check["required"] == true && check["passed"] != true {
			ready = false
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": ready, "checks": checks, "freeBytes": freeBytes, "imagePath": imagePath, "imageSizeBytes": manifest.PackageSize, "activeSlot": state.ActiveSlot, "pendingSlot": state.PendingSlot})
}

// slotDeviceMapping is the appliance's slot layout: which device and EFI
// boot entry belong to each slot, e.g.
//
//	LUMONAS_SLOT_DEVICES=a=/dev/disk/by-partlabel/lumonas-a:0001,b=/dev/disk/by-partlabel/lumonas-b:0002
type slotDeviceMapping map[string]slotTarget

type slotTarget struct {
	Device    string
	BootEntry string
}

func parseSlotDeviceMapping(raw string) (slotDeviceMapping, error) {
	mapping := slotDeviceMapping{}
	if strings.TrimSpace(raw) == "" {
		return mapping, errors.New("slot device mapping is not configured (LUMONAS_SLOT_DEVICES)")
	}
	for _, part := range strings.Split(raw, ",") {
		slot, rest, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, errors.New("slot device mapping is invalid")
		}
		slot = strings.ToLower(strings.TrimSpace(slot))
		if slot != "a" && slot != "b" {
			return nil, errors.New("slot device mapping is invalid")
		}
		if _, exists := mapping[slot]; exists {
			return nil, errors.New("slot device mapping contains a duplicate slot")
		}
		device, entry, ok := strings.Cut(rest, ":")
		device = strings.TrimSpace(device)
		if !ok || updates.ValidateSlotDevicePath(device) != nil {
			return nil, errors.New("slot device mapping is invalid")
		}
		if err := updates.ValidateSlotBootEntry(strings.TrimSpace(entry)); err != nil {
			return nil, errors.New("slot device mapping is invalid")
		}
		mapping[slot] = slotTarget{Device: device, BootEntry: strings.TrimSpace(entry)}
	}
	if len(mapping) != 2 {
		return nil, errors.New("slot device mapping must define both slots")
	}
	return mapping, nil
}

// stageSlotImage stages a signed root-filesystem image for the inactive
// slot. The image must already be on the appliance (fetched by the update
// channel); only its path, manifest, and signature are supplied here.
func (s *apiServer) stageSlotImage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ImagePath string           `json:"imagePath"`
		Manifest  updates.Manifest `json:"manifest"`
		Signature string           `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	publicKey, err := updates.ParsePublicKey(osUpdatePublicKey())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	signature, err := updates.ParseSignature(input.Signature)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.ImagePath) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "imagePath is required"})
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is required before installing an update"})
		return
	}
	backupPath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	bundle, err := os.ReadFile(backupPath)
	if err != nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "a verified recovery bundle is required before installing an update"})
		return
	}
	if _, err := recovery.Verify(bundle, []byte(key)); err != nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "pre-update recovery verification failed"})
		return
	}
	state, err := s.updateManager().StageSlotImage(input.ImagePath, input.Manifest, signature, publicKey)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "updates.slot.stage", "slot-"+state.PendingSlot, map[string]any{"version": state.PendingVersion, "slot": state.PendingSlot})
	s.publish("updates.slot.staged", "info", &model.ResourceRef{Type: "update", ID: state.PendingVersion}, map[string]any{"slot": state.PendingSlot, "version": state.PendingVersion})
	writeJSON(w, http.StatusAccepted, state)
}

func osUpdatePublicKey() string {
	return os.Getenv("LUMONAS_UPDATE_PUBLIC_KEY")
}

// rebootToPreviousSlot is only active when the appliance has an explicit
// A/B device mapping. It arms the known-good EFI entry first and requests a
// reboot only after that privileged operation succeeds; a missing mapping or
// failed broker call therefore fails closed without a blind reboot.
func (s *apiServer) rebootToPreviousSlot(state updates.SlotState) {
	mapping, err := parseSlotDeviceMapping(os.Getenv("LUMONAS_SLOT_DEVICES"))
	if err != nil {
		if s.log != nil {
			s.log.Warn("automatic slot rollback is not configured", "error", err)
		}
		return
	}
	target, ok := mapping[state.ActiveSlot]
	if !ok {
		if s.log != nil {
			s.log.Error("automatic slot rollback has no active-slot mapping", "slot", state.ActiveSlot)
		}
		return
	}
	operationID := newID("slot-rollback")
	planHash := newID("slot-rollback-plan")
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelBoot()
	bootResult, bootErr := s.executePrivileged(bootCtx, privileged.Request{
		Operation:   "system.slot.bootnext",
		OperationID: operationID,
		PlanHash:    planHash,
		RequestedState: map[string]any{
			"entry": target.BootEntry,
		},
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
		Confirmed: true,
	})
	if bootErr != nil || !bootResult.OK {
		if s.log != nil {
			s.log.Error("automatic slot rollback could not arm BootNext", "error", firstError(bootErr, bootResult.Error), "slot", state.ActiveSlot)
		}
		return
	}
	rebootResult, rebootErr := s.executePrivileged(bootCtx, privileged.Request{
		Operation:   "power.shutdown",
		OperationID: operationID,
		PlanHash:    planHash,
		RequestedState: map[string]any{
			"action": "reboot",
		},
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
		Confirmed: true,
	})
	if rebootErr != nil || !rebootResult.OK {
		if s.log != nil {
			s.log.Error("automatic slot rollback could not reboot", "error", firstError(rebootErr, rebootResult.Error), "slot", state.ActiveSlot)
		}
		return
	}
	if s.log != nil {
		s.log.Warn("automatic slot rollback armed", "slot", state.ActiveSlot, "bootNext", target.BootEntry, "operationId", operationID)
	}
}

func firstError(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

// activateSlotImage writes the staged image to the inactive slot device and
// arms BootNext. Both privileged steps carry the same operation ID so the
// audit trail shows one activation.
func (s *apiServer) activateSlotImage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	mapping, err := parseSlotDeviceMapping(os.Getenv("LUMONAS_SLOT_DEVICES"))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	publicKey, keyErr := updates.ParsePublicKey(osUpdatePublicKey())
	if keyErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": keyErr.Error()})
		return
	}
	imagePath, manifest, state, err := s.updateManager().StagedSlotImageVerified(publicKey)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	target, ok := mapping[state.PendingSlot]
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "slot device mapping is incomplete"})
		return
	}
	operationID := newID("slot")
	request := privileged.Request{
		Operation:   "system.slot.write",
		OperationID: operationID,
		PlanHash:    manifest.PackageSHA256,
		Confirmed:   true,
		ExpiresAt:   time.Now().UTC().Add(45 * time.Minute),
		RequestedState: map[string]any{
			"imagePath":      imagePath,
			"targetDevice":   target.Device,
			"expectedDigest": manifest.PackageSHA256,
		},
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Minute)
	defer cancel()
	if err := s.brokerExecute(ctx, request); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	bootNext := privileged.Request{
		Operation:   "system.slot.bootnext",
		OperationID: operationID,
		PlanHash:    manifest.PackageSHA256,
		Confirmed:   true,
		ExpiresAt:   time.Now().UTC().Add(2 * time.Minute),
		RequestedState: map[string]any{
			"entry": target.BootEntry,
		},
	}
	bootCtx, cancelBoot := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancelBoot()
	if err := s.brokerExecute(bootCtx, bootNext); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "updates.slot.activate", "slot-"+state.PendingSlot, map[string]any{"version": state.PendingVersion, "device": target.Device, "bootNext": target.BootEntry})
	s.publish("updates.slot.activated", "info", &model.ResourceRef{Type: "update", ID: state.PendingVersion}, map[string]any{"slot": state.PendingSlot, "version": state.PendingVersion})
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "activated", "slot": state.PendingSlot, "version": state.PendingVersion, "bootNext": target.BootEntry})
}

// confirmSlotImage promotes the slot the appliance just booted from, after
// the running version proves the new image is healthy.
func (s *apiServer) confirmSlotImage(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	state, err := s.updateManager().CommitSlotImage(s.version)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "updates.slot.confirm", "slot-"+state.ActiveSlot, map[string]any{"version": state.ActiveVersion})
	s.publish("updates.slot.committed", "info", &model.ResourceRef{Type: "update", ID: state.ActiveVersion}, map[string]any{"slot": state.ActiveSlot, "version": state.ActiveVersion})
	writeJSON(w, http.StatusOK, state)
}
