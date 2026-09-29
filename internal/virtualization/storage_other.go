//go:build !linux

package virtualization

import "os"

func availableStorageBytes(string) (uint64, error) {
	return ^uint64(0), nil
}

func allocatedFileBytes(info os.FileInfo) (uint64, error) {
	if info.Size() < 0 {
		return 0, os.ErrInvalid
	}
	return uint64(info.Size()), nil
}

func prepareQemuBackupDirectory(string) error { return nil }

func prepareQemuBackupFile(string) error { return nil }
