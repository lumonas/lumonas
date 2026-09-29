package updates

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Slot-image support: the immutable-OS upgrade path. A staged slot image is
// a verified root-filesystem image for the inactive slot; activation writes
// it to the slot device through the privileged broker and flips the
// bootloader's BootNext entry. Health gating and rollback reuse the existing
// boot-attempt machinery in SlotState.

// StageSlotImage verifies a signed root-filesystem image and stages it under
// the inactive slot directory together with its manifest. Nothing is written
// to any device here — that is the privileged broker's job once the operator
// activates the slot.
func (m *Manager) StageSlotImage(imagePath string, manifest Manifest, signature []byte, publicKey ed25519.PublicKey) (SlotState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := VerifyManifest(publicKey, manifest, signature); err != nil {
		return SlotState{}, err
	}
	if err := VerifyPackage(imagePath, manifest); err != nil {
		return SlotState{}, err
	}
	if err := os.MkdirAll(m.Root, 0o750); err != nil {
		return SlotState{}, fmt.Errorf("create update root: %w", err)
	}
	state, err := m.loadLocked()
	if err != nil {
		return SlotState{}, err
	}
	if state.ActiveSlot == "" {
		state.ActiveSlot = "a"
	}
	inactive := "b"
	if state.ActiveSlot == "b" {
		inactive = "a"
	}
	slotDir := filepath.Join(m.Root, "slot-"+inactive)
	if err := os.RemoveAll(slotDir); err != nil {
		return SlotState{}, fmt.Errorf("clear inactive slot: %w", err)
	}
	if err := os.MkdirAll(slotDir, 0o750); err != nil {
		return SlotState{}, fmt.Errorf("create inactive slot: %w", err)
	}
	if err := copyFile(imagePath, filepath.Join(slotDir, "image")); err != nil {
		return SlotState{}, fmt.Errorf("stage slot image: %w", err)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return SlotState{}, fmt.Errorf("encode staged slot manifest: %w", err)
	}
	if err := atomicWrite(filepath.Join(slotDir, "image-manifest.json"), encoded, 0o640); err != nil {
		return SlotState{}, fmt.Errorf("write staged slot manifest: %w", err)
	}
	if err := atomicWrite(filepath.Join(slotDir, "image-signature"), []byte(base64.StdEncoding.EncodeToString(signature)), 0o640); err != nil {
		return SlotState{}, fmt.Errorf("write staged slot signature: %w", err)
	}
	state.PreviousSlot = state.ActiveSlot
	state.PendingSlot = inactive
	state.PendingVersion = manifest.Version
	state.BootAttempts = 0
	state.LastError = ""
	state.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(state); err != nil {
		return SlotState{}, err
	}
	return state, nil
}

// ErrNoStagedSlotImage reports that activation was requested without a
// staged image.
var ErrNoStagedSlotImage = errors.New("no slot image is staged for the inactive slot")

// StagedSlotImage resolves the staged image for the inactive slot and
// re-verifies it against the persisted manifest, so drift between staging
// and activation cannot reach the slot device. The returned path is what the
// privileged broker is asked to write.
func (m *Manager) StagedSlotImage() (string, Manifest, SlotState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.loadLocked()
	if err != nil {
		return "", Manifest{}, state, err
	}
	if state.PendingSlot == "" || state.PendingVersion == "" {
		return "", Manifest{}, state, ErrNoStagedSlotImage
	}
	slotDir := filepath.Join(m.Root, "slot-"+state.PendingSlot)
	manifestData, err := os.ReadFile(filepath.Join(slotDir, "image-manifest.json"))
	if err != nil {
		return "", Manifest{}, state, fmt.Errorf("read staged slot manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", Manifest{}, state, fmt.Errorf("parse staged slot manifest: %w", err)
	}
	imagePath := filepath.Join(slotDir, "image")
	if err := VerifyPackage(imagePath, manifest); err != nil {
		return "", Manifest{}, state, fmt.Errorf("staged slot image verification failed: %w", err)
	}
	return imagePath, manifest, state, nil
}

func (m *Manager) StagedSlotImageVerified(publicKey ed25519.PublicKey) (string, Manifest, SlotState, error) {
	imagePath, manifest, state, err := m.StagedSlotImage()
	if err != nil {
		return "", Manifest{}, state, err
	}
	signatureData, err := os.ReadFile(filepath.Join(m.Root, "slot-"+state.PendingSlot, "image-signature"))
	if err != nil {
		return "", Manifest{}, state, fmt.Errorf("read staged slot signature: %w", err)
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signatureData)))
	if err != nil {
		return "", Manifest{}, state, fmt.Errorf("parse staged slot signature: %w", err)
	}
	if err := VerifyManifest(publicKey, manifest, signature); err != nil {
		return "", Manifest{}, state, fmt.Errorf("staged slot signature verification failed: %w", err)
	}
	return imagePath, manifest, state, nil
}

// CommitSlotImage promotes the pending slot to active after the appliance
// reported healthy from it. The device plumbing (BootNext entry) is the
// broker's concern; this only settles the persisted state.
func (m *Manager) CommitSlotImage(version string) (SlotState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.loadLocked()
	if err != nil {
		return SlotState{}, err
	}
	if state.PendingSlot == "" {
		return SlotState{}, ErrNoStagedSlotImage
	}
	if strings.TrimSpace(version) == "" || version != state.PendingVersion {
		return SlotState{}, fmt.Errorf("slot health version does not match pending version")
	}
	state.ActiveSlot = state.PendingSlot
	state.ActiveVersion = version
	state.PendingSlot = ""
	state.PendingVersion = ""
	state.BootAttempts = 0
	state.LastError = ""
	state.UpdatedAt = time.Now().UTC()
	return state, m.saveLocked(state)
}

// ValidateSlotBootEntry checks a bootloader entry number for the BootNext
// privileged operation.
func ValidateSlotBootEntry(entry string) error {
	if len(entry) == 0 || len(entry) > 4 {
		return errors.New("boot entry is invalid")
	}
	for _, char := range entry {
		isHex := (char >= '0' && char <= '9') || (char >= 'A' && char <= 'F') || (char >= 'a' && char <= 'f')
		if !isHex {
			return errors.New("boot entry is invalid")
		}
	}
	return nil
}

// ValidateSlotDevicePath accepts only persistent udev aliases for an OS slot.
// Kernel names such as /dev/sda and /dev/vdb can change when disks are
// reordered, so allowing them at the privileged image-writing boundary would
// defeat the A/B safety contract.
func ValidateSlotDevicePath(device string) error {
	device = strings.TrimSpace(device)
	if device == "" || filepath.Clean(device) != device || strings.Contains(device, "..") {
		return errors.New("slot device path is invalid")
	}
	for _, prefix := range []string{"/dev/disk/by-id/", "/dev/disk/by-partlabel/"} {
		if strings.HasPrefix(device, prefix) && len(device) > len(prefix) {
			return nil
		}
	}
	return errors.New("slot device must use a persistent /dev/disk/by-* alias")
}
