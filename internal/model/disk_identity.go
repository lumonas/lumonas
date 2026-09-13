package model

import "strings"

// HasStableDiskIdentity reports whether a disk ID is safe to bind into a
// persisted or mutating operation plan. A kernel-path fallback is useful for
// read-only diagnostics, but it can identify a different device after a
// replacement at the same /dev path.
func HasStableDiskIdentity(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && !strings.HasPrefix(id, "path:")
}
