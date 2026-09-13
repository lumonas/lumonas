package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/storage"
)

// applySnapraidConfig renders and atomically writes the managed snapraid
// configuration from stable disk identities. The config path must be inside
// the allow-listed SnapRAID config locations and the rendered content only
// ever references canonical /srv/disks branch paths, so no unvalidated input
// reaches the filesystem.
func applySnapraidConfig(req request, discover func(collector.CommandRunner) ([]model.Disk, error), run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	configPath := requestedString(req.RequestedState, "configPath")
	if configPath == "" {
		configPath = "/etc/lumonas/snapraid.conf"
	}
	if !safeSnapraidConfig(configPath) {
		return response{Error: "SnapRAID config path is not allow-listed"}
	}
	parity := requestedString(req.RequestedState, "parityDiskId")
	data := requestedStrings(req.RequestedState, "dataDiskIds")
	// Replacement flows supply an explicit name→disk mapping so the retired
	// disk's data slot keeps its name; regular flows use sorted identities.
	slots, pinned := requestedDataSlots(req.RequestedState, "dataSlots")
	var content string
	var err error
	if pinned {
		content, err = storage.RenderSnapraidConfigPinned(parity, slots)
		if err == nil {
			data = make([]string, 0, len(slots))
			for _, slot := range slots {
				data = append(data, slot.DiskID)
			}
		}
	} else {
		content, err = storage.RenderSnapraidConfig(parity, data)
	}
	if err != nil {
		return response{Error: err.Error()}
	}
	if err := storage.ValidateSnapraidConfig(content); err != nil {
		return response{Error: err.Error()}
	}
	if err := revalidateSnapraidDisks(req, parity, data, discover); err != nil {
		return response{Error: err.Error()}
	}
	return activateSnapraidConfig(configPath, content, run)
}

func activateSnapraidConfig(configPath, content string, run command) response {
	if existing, readErr := os.ReadFile(configPath); readErr == nil && string(existing) == content {
		return response{OK: true, Data: map[string]string{"configPath": configPath, "state": "unchanged"}}
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return response{Error: "SnapRAID config directory could not be created"}
	}
	temporary, err := os.CreateTemp(filepath.Dir(configPath), ".snapraid-conf-*")
	if err != nil {
		return response{Error: "SnapRAID config could not be written"}
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return response{Error: "SnapRAID config could not be written"}
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return response{Error: "SnapRAID config could not be synced"}
	}
	if err := temporary.Close(); err != nil {
		return response{Error: "SnapRAID config could not be written"}
	}
	if err := os.Chmod(temporaryPath, 0o640); err != nil {
		return response{Error: "SnapRAID config could not be written"}
	}
	if _, err := run("snapraid", "-c", temporaryPath, "status"); err != nil {
		return response{Error: "SnapRAID config validation failed"}
	}
	if err := os.Rename(temporaryPath, configPath); err != nil {
		return response{Error: "SnapRAID config could not be activated"}
	}
	return response{OK: true, Data: map[string]string{"configPath": configPath, "state": "active"}}
}

func revalidateSnapraidDisks(req request, parity string, data []string, discover func(collector.CommandRunner) ([]model.Disk, error)) error {
	if len(req.ExpectedDisks) == 0 {
		return fmt.Errorf("expected SnapRAID disk identities are required")
	}
	if discover == nil {
		return fmt.Errorf("disk identity discovery is unavailable")
	}
	requested := make(map[string]bool, len(data)+1)
	if parity != "" {
		requested[parity] = true
	}
	for _, id := range data {
		requested[id] = true
	}
	if len(requested) != len(req.ExpectedDisks) {
		return fmt.Errorf("expected SnapRAID disks do not match requested layout")
	}
	actual, err := discover(nil)
	if err != nil {
		return fmt.Errorf("disk identity discovery failed")
	}
	byID := make(map[string]model.Disk, len(actual))
	for _, disk := range actual {
		byID[disk.ID] = disk
	}
	expected := append([]expectedDisk(nil), req.ExpectedDisks...)
	sort.Slice(expected, func(i, j int) bool { return expected[i].ID < expected[j].ID })
	for _, disk := range expected {
		if !model.HasStableDiskIdentity(disk.ID) {
			return fmt.Errorf("SnapRAID disk %q has no stable identity", disk.ID)
		}
		if !requested[disk.ID] {
			return fmt.Errorf("expected SnapRAID disk %q is not part of the requested layout", disk.ID)
		}
		actualDisk, ok := byID[disk.ID]
		if !ok {
			return fmt.Errorf("SnapRAID disk %q is no longer present", disk.ID)
		}
		identity := map[string]string{"id": disk.ID, "wwn": disk.WWN, "serial": disk.Serial, "model": disk.Model, "gptDiskGuid": disk.GPTDiskGUID, "partitionUuid": disk.PartitionUUID, "filesystemUuid": disk.FilesystemUUID}
		if disk.SizeBytes != 0 {
			identity["sizeBytes"] = fmt.Sprint(disk.SizeBytes)
		}
		if err := validateIdentity(actualDisk, identity); err != nil {
			return fmt.Errorf("SnapRAID disk %q changed: %w", disk.ID, err)
		}
	}
	return nil
}
