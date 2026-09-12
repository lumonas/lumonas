package acl

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type Entry struct {
	Principal string `json:"principal"`
	Level     string `json:"level"`
}

var principalPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,31}$`)

func Validate(path string, entries []Entry, recursive bool) error {
	clean := filepath.Clean(path)
	if clean != path || !IsSafePath(clean) {
		return errors.New("ACL path is not allow-listed")
	}
	if len(entries) == 0 || len(entries) > 1000 {
		return errors.New("ACL entries must contain between 1 and 1000 items")
	}
	_ = recursive // kept in the signature so callers validate one typed request
	seen := map[string]bool{}
	for _, entry := range entries {
		if !principalPattern.MatchString(entry.Principal) {
			return fmt.Errorf("ACL principal %q is invalid", entry.Principal)
		}
		if seen[entry.Principal] {
			return fmt.Errorf("ACL principal %q is listed more than once", entry.Principal)
		}
		seen[entry.Principal] = true
		switch entry.Level {
		case "none", "read", "write":
		default:
			return fmt.Errorf("ACL level %q is invalid", entry.Level)
		}
	}
	return nil
}

func IsSafePath(path string) bool {
	return strings.HasPrefix(path, "/srv/disks/") || strings.HasPrefix(path, "/srv/pools/")
}
