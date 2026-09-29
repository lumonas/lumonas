//go:build linux

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func IsRemovableBackupTarget(target string) bool {
	target = filepath.Clean(target)
	for _, root := range []string{"/media", "/mnt", "/run/media"} {
		if relative, err := filepath.Rel(root, target); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func RemovableBackupTargetMounted(target string) bool {
	return IsRemovableBackupTarget(target) && requireMountedBackupTarget(target) == nil
}

// requireMountedBackupTarget prevents an unplugged USB disk mounted below
// /media or /mnt from silently receiving backups on the NAS root filesystem.
func requireMountedBackupTarget(target string) error {
	target = filepath.Clean(target)
	managedRoot := ""
	for _, root := range []string{"/media", "/mnt", "/run/media"} {
		if relative, err := filepath.Rel(root, target); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			managedRoot = root
			break
		}
	}
	if managedRoot == "" {
		return nil
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return errors.New("cannot verify that the removable backup destination is mounted")
	}
	return requireMountedBackupTargetFrom(target, string(data))
}

func requireMountedBackupTargetFrom(target, mountInfo string) error {
	target = filepath.Clean(target)
	managedRoot := ""
	for _, root := range []string{"/media", "/mnt", "/run/media"} {
		if relative, err := filepath.Rel(root, target); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			managedRoot = root
			break
		}
	}
	if managedRoot == "" {
		return nil
	}
	best := ""
	for _, line := range strings.Split(mountInfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		mountPoint := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(fields[4])
		mountPoint = filepath.Clean(mountPoint)
		relative, relErr := filepath.Rel(mountPoint, target)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if len(mountPoint) > len(best) {
			best = mountPoint
		}
	}
	if best == "" || (best != managedRoot && !strings.HasPrefix(best, managedRoot+string(filepath.Separator))) {
		return errors.New("removable backup destination is not mounted; backup was not written to the NAS system disk")
	}
	return nil
}
