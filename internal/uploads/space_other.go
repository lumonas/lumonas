//go:build !linux

package uploads

import "errors"

// availableBytes has no portable equivalent outside Linux, so callers fall back
// to the filesystem's own write errors rather than pre-checking capacity.
func availableBytes(string) (int64, error) {
	return 0, errors.ErrUnsupported
}
