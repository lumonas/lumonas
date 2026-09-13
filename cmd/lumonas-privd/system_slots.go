package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	commandrunner "github.com/lumonas/lumonas/internal/runner"
	"github.com/lumonas/lumonas/internal/updates"
)

// slotWriteTimeout bounds one slot image write; a multi-gigabyte root image
// takes minutes on real media.
var slotWriteTimeout = 30 * time.Minute

// slotTargetStat is injectable so unit tests can exercise the mount and
// digest guards without creating a real block device. Production always uses
// os.Stat.
var slotTargetStat = os.Stat

// slotUpdateRoot is the only directory slot images may be staged in.
func slotUpdateRoot() string {
	return envOr("LUMONAS_UPDATE_ROOT", "/var/lib/lumonas/updates")
}

// applySlotWrite copies a verified slot image onto the inactive slot device.
// The image must live inside the appliance update root and must match the
// digest the API recorded at staging time; the target must be an unmounted
// block device.
func applySlotWrite(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	imagePath := requestedString(req.RequestedState, "imagePath")
	target := requestedString(req.RequestedState, "targetDevice")
	expectedDigest := requestedString(req.RequestedState, "expectedDigest")

	root := filepath.Clean(slotUpdateRoot())
	cleanImage := filepath.Clean(imagePath)
	if !filepath.IsAbs(cleanImage) || (cleanImage != root && !strings.HasPrefix(cleanImage, root+string(filepath.Separator))) {
		return response{Error: "slot image must live inside the update root"}
	}
	info, err := os.Stat(cleanImage)
	if err != nil {
		return response{Error: "staged slot image is missing"}
	}
	if !info.Mode().IsRegular() {
		return response{Error: "slot image must be a regular file"}
	}
	// Digest first: it validates the payload without depending on the
	// platform's device layout, so a corrupted image is rejected before any
	// device-specific checks.
	digest, err := fileDigest(cleanImage)
	if err != nil {
		return response{Error: "slot image could not be read"}
	}
	if !strings.EqualFold(digest, expectedDigest) {
		return response{Error: "slot image digest does not match the staged manifest"}
	}
	// Write access to arbitrary devices is the point of this operation, so
	// the target is checked as strictly as the image path: block device,
	// not mounted, no loop leftovers.
	if !strings.HasPrefix(target, "/dev/") || strings.Contains(target, "..") {
		return response{Error: "slot target must be a device under /dev"}
	}
	if targetStat, statErr := slotTargetStat(target); statErr != nil || targetStat.Mode()&os.ModeDevice == 0 || targetStat.Mode()&os.ModeCharDevice != 0 {
		return response{Error: "slot target is not a block device"}
	}
	if mounted, mountErr := deviceHasMounts(target, run); mountErr != nil {
		return response{Error: "could not verify slot target mount state"}
	} else if mounted {
		return response{Error: "slot target is mounted"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), slotWriteTimeout)
	defer cancel()
	if _, err := commandrunner.CombinedOutputContext(ctx, "dd", "if="+cleanImage, "of="+target, "bs=4M", "conv=fsync", "status=none"); err != nil {
		return response{Error: "slot image write failed"}
	}
	return response{OK: true, Data: map[string]any{
		"target":       target,
		"image":        cleanImage,
		"sizeBytes":    info.Size(),
		"digestSha256": digest,
	}}
}

// applySlotBootNext arms the bootloader's one-shot BootNext entry so the
// next reboot starts the freshly written slot. efibootmgr is required; on
// BIOS-only systems the operation fails closed.
func applySlotBootNext(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	entry := requestedString(req.RequestedState, "entry")
	if err := updates.ValidateSlotBootEntry(entry); err != nil {
		return response{Error: err.Error()}
	}
	if _, err := run("efibootmgr", "--bootnext", entry); err != nil {
		return response{Error: "BootNext could not be armed"}
	}
	return response{OK: true, Data: map[string]string{"bootNext": entry}}
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash slot image: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

var errSlotDeviceMissing = errors.New("slot target device is missing")
