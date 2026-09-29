//go:build !linux

package backup

func IsRemovableBackupTarget(string) bool      { return false }
func RemovableBackupTargetMounted(string) bool { return false }
func requireMountedBackupTarget(string) error  { return nil }
