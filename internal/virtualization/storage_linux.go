//go:build linux

package virtualization

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func availableStorageBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}

func allocatedFileBytes(info os.FileInfo) (uint64, error) {
	if info.Size() < 0 {
		return 0, os.ErrInvalid
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return uint64(info.Size()), nil
	}
	return uint64(stat.Blocks) * 512, nil
}

func prepareQemuBackupDirectory(path string) error {
	group, err := user.LookupGroup("libvirt")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}
	if err := os.Chown(path, -1, gid); err != nil {
		return err
	}
	return os.Chmod(path, 0770)
}

func prepareQemuBackupFile(path string) error {
	group, err := user.LookupGroup("libvirt")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}
	if err := os.Chown(path, -1, gid); err != nil {
		return err
	}
	return os.Chmod(path, 0660)
}
