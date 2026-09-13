package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
)

// slotTestDiscover satisfies execute's signature; slot operations resolve
// their target by device path and never consult the disk inventory.
func slotTestDiscover(_ collector.CommandRunner) ([]model.Disk, error) {
	return nil, nil
}

func slotWriteRequest(imagePath, target, digest string) request {
	return request{
		Operation:   "system.slot.write",
		OperationID: "slot-write-1",
		PlanHash:    "slot-plan",
		Confirmed:   true,
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
		RequestedState: map[string]any{
			"imagePath":      imagePath,
			"targetDevice":   target,
			"expectedDigest": digest,
		},
	}
}

func stagedSlotImage(t *testing.T, payload string) (string, string) {
	t.Helper()
	root := t.TempDir()
	// The image must live inside the update root; mirror the real layout.
	updateRoot := filepath.Join(root, "updates", "slot-b")
	if err := os.MkdirAll(updateRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_UPDATE_ROOT", filepath.Join(root, "updates"))
	imagePath := filepath.Join(updateRoot, "image")
	if err := os.WriteFile(imagePath, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(payload))
	return imagePath, hex.EncodeToString(digest[:])
}

type fakeBlockFileInfo struct{}

func (fakeBlockFileInfo) Name() string       { return "slot-device" }
func (fakeBlockFileInfo) Size() int64        { return 0 }
func (fakeBlockFileInfo) Mode() os.FileMode  { return os.ModeDevice }
func (fakeBlockFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeBlockFileInfo) IsDir() bool        { return false }
func (fakeBlockFileInfo) Sys() any           { return nil }

type fakeCharDeviceFileInfo struct{}

func (fakeCharDeviceFileInfo) Name() string       { return "slot-character-device" }
func (fakeCharDeviceFileInfo) Size() int64        { return 0 }
func (fakeCharDeviceFileInfo) Mode() os.FileMode  { return os.ModeDevice | os.ModeCharDevice }
func (fakeCharDeviceFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeCharDeviceFileInfo) IsDir() bool        { return false }
func (fakeCharDeviceFileInfo) Sys() any           { return nil }

type fakeRegularFileInfo struct{}

func (fakeRegularFileInfo) Name() string       { return "slot-regular-file" }
func (fakeRegularFileInfo) Size() int64        { return 0 }
func (fakeRegularFileInfo) Mode() os.FileMode  { return 0 }
func (fakeRegularFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeRegularFileInfo) IsDir() bool        { return false }
func (fakeRegularFileInfo) Sys() any           { return nil }

func allowFakeSlotBlockDevice(t *testing.T) {
	t.Helper()
	previous := slotTargetStat
	slotTargetStat = func(string) (os.FileInfo, error) { return fakeBlockFileInfo{}, nil }
	t.Cleanup(func() { slotTargetStat = previous })
}

func TestSlotWriteRejectsOutsideUpdateRoot(t *testing.T) {
	imagePath, digest := stagedSlotImage(t, "payload")
	run := func(name string, args ...string) ([]byte, error) {
		return nil, nil
	}
	_ = imagePath
	outside := filepath.Join(t.TempDir(), "elsewhere.img")
	if err := os.WriteFile(outside, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := execute(slotWriteRequest(outside, "/dev/vdb", digest), slotTestDiscover, run)
	if result.OK || !strings.Contains(result.Error, "update root") {
		t.Fatalf("expected update-root rejection, got %#v", result)
	}
}

func TestSlotWriteRejectsNonBlockTarget(t *testing.T) {
	imagePath, digest := stagedSlotImage(t, "payload")
	previous := slotTargetStat
	slotTargetStat = func(string) (os.FileInfo, error) { return fakeRegularFileInfo{}, nil }
	t.Cleanup(func() { slotTargetStat = previous })
	result := execute(slotWriteRequest(imagePath, "/dev/disk/by-id/virtio-LUMONAS-SLOTB", digest), slotTestDiscover, func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "block device") {
		t.Fatalf("expected target rejection, got %#v", result)
	}
}

func TestSlotWriteRejectsPartitionTarget(t *testing.T) {
	imagePath, digest := stagedSlotImage(t, "payload")
	allowFakeSlotBlockDevice(t)
	result := execute(slotWriteRequest(imagePath, "/dev/disk/by-id/virtio-LUMONAS-SLOTB", digest), slotTestDiscover, func(name string, _ ...string) ([]byte, error) {
		if name == "lsblk" {
			return []byte("part\n"), nil
		}
		return []byte(""), nil
	})
	if result.OK || !strings.Contains(result.Error, "whole disk") {
		t.Fatalf("expected partition rejection, got %#v", result)
	}
}

func TestSlotWriteVerifiesDigestBeforeWrite(t *testing.T) {
	imagePath, _ := stagedSlotImage(t, "payload")
	allowFakeSlotBlockDevice(t)
	// A block-device-looking path cannot exist in tests, so the digest check
	// is proven independently via fileDigest on a real loop-style file.
	digest, err := fileDigest(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if digest != sha256Hex("payload") {
		t.Fatalf("unexpected digest %s", digest)
	}

	result := execute(slotWriteRequest(imagePath, "/dev/disk/by-id/virtio-LUMONAS-SLOTB", strings.Repeat("0", 64)), slotTestDiscover, func(name string, _ ...string) ([]byte, error) {
		// findmnt probe: target has no mounts.
		return []byte(""), nil
	})
	if result.OK || !strings.Contains(result.Error, "digest") {
		t.Fatalf("expected digest mismatch before any write, got %#v", result)
	}
}

func TestSlotWriteFailsClosedWhenMountStateUnknown(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the slot target must be a real block device; Linux-only")
	}
	imagePath, digest := stagedSlotImage(t, "payload")
	allowFakeSlotBlockDevice(t)
	result := execute(slotWriteRequest(imagePath, "/dev/disk/by-id/virtio-LUMONAS-SLOTB", digest), slotTestDiscover, func(name string, _ ...string) ([]byte, error) {
		if name == "findmnt" || name == "lsblk" {
			return nil, fmt.Errorf("findmnt unavailable")
		}
		return []byte(""), nil
	})
	if result.OK || !strings.Contains(result.Error, "could not verify") {
		t.Fatalf("expected fail-closed mount probe, got %#v", result)
	}
}

func TestSlotBootNextValidatesEntryAndRequiresEfibootmgr(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	request := request{
		Operation:      "system.slot.bootnext",
		OperationID:    "slot-boot-1",
		PlanHash:       "slot-plan",
		Confirmed:      true,
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
		RequestedState: map[string]any{"entry": "0002"},
	}
	result := execute(request, slotTestDiscover, run)
	if !result.OK {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	if len(commands) != 1 || commands[0] != "efibootmgr --bootnext 0002" {
		t.Fatalf("unexpected bootnext command: %v", commands)
	}

	request.RequestedState["entry"] = "drop table"
	if result := execute(request, slotTestDiscover, run); result.OK {
		t.Fatal("invalid entry must be rejected")
	}
}

func TestSlotOperationsAreMutationsOnStorageWorker(t *testing.T) {
	if !requiresOperationID("system.slot.write") || !requiresOperationID("system.slot.bootnext") {
		t.Fatal("slot operations must require an operation ID")
	}
	if operationWorker("system.slot.write") != "storage" || operationWorker("system.slot.bootnext") != "storage" {
		t.Fatal("slot operations must route to the storage worker")
	}
}

func TestSlotWriteRejectsCharacterDevice(t *testing.T) {
	imagePath, digest := stagedSlotImage(t, "payload")
	previous := slotTargetStat
	slotTargetStat = func(string) (os.FileInfo, error) { return fakeCharDeviceFileInfo{}, nil }
	t.Cleanup(func() { slotTargetStat = previous })
	result := execute(slotWriteRequest(imagePath, "/dev/disk/by-id/virtio-LUMONAS-SLOTB", digest), slotTestDiscover, func(string, ...string) ([]byte, error) {
		return []byte(""), nil
	})
	if result.OK || !strings.Contains(result.Error, "slot target") {
		t.Fatalf("expected character-device rejection, got %#v", result)
	}
}

func TestExecuteSlotWriteFailsClosedOnMountDiscovery(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the slot target must be a real block device; Linux-only")
	}
	imagePath, digest := stagedSlotImage(t, "payload")
	allowFakeSlotBlockDevice(t)
	disk := model.Disk{ID: "wwn:slot", CurrentPath: "/dev/vdb", SizeBytes: 32 << 30, WWN: "wwn:slot"}
	req := request{
		Operation: "system.slot.write", OperationID: "slot-3", PlanHash: "plan",
		TargetDiskID: disk.ID, Confirmed: true,
		ExpiresAt:        time.Now().UTC().Add(time.Minute),
		ExpectedIdentity: map[string]string{"wwn": disk.WWN},
		RequestedState: map[string]any{
			"imagePath": imagePath, "targetDevice": "/dev/disk/by-id/virtio-LUMONAS-SLOTB", "expectedDigest": digest,
		},
	}
	result := execute(req, func(_ collector.CommandRunner) ([]model.Disk, error) {
		return []model.Disk{disk}, nil
	}, func(name string, _ ...string) ([]byte, error) {
		if name == "findmnt" || name == "lsblk" {
			return nil, fmt.Errorf("findmnt unavailable")
		}
		return []byte(""), nil
	})
	if result.OK || !strings.Contains(result.Error, "could not verify") {
		t.Fatalf("expected fail-closed mount probe, got %#v", result)
	}
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
