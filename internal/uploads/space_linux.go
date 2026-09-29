//go:build linux

package uploads

import "golang.org/x/sys/unix"

// availableBytes reports the free space available to an unprivileged writer on
// the filesystem holding path. A negative result means the filesystem does not
// support the query, and callers must treat it as "unknown" rather than "full".
func availableBytes(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Bavail is the count available to this user; Bfree includes the root
	// reserve that the daemon cannot consume.
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
