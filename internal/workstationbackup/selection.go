package workstationbackup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"path/filepath"
	"strings"
)

// Selection limits which paths from a workstation source folder enter a backup.
// Paths and glob patterns are relative to the source root and use forward slashes.
type Selection struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

func (selection Selection) Validate() error {
	if len(selection.Include) > 256 || len(selection.Exclude) > 256 {
		return errors.New("at most 256 include and 256 exclude patterns are allowed")
	}
	for _, pattern := range append(append([]string(nil), selection.Include...), selection.Exclude...) {
		if pattern == "" || len(pattern) > 512 || strings.ContainsAny(pattern, "\\\x00\r\n") || strings.HasPrefix(pattern, "/") || filepath.IsAbs(pattern) || path.Clean(pattern) != pattern || pattern == "." {
			return errors.New("selection patterns must be clean relative paths or globs")
		}
		for _, part := range strings.Split(pattern, "/") {
			if part == ".." {
				return errors.New("selection patterns cannot traverse outside the source folder")
			}
		}
		if _, err := path.Match(pattern, "selection-probe"); err != nil {
			return errors.New("selection pattern is not a valid glob")
		}
	}
	return nil
}

func (selection Selection) hash() string {
	if len(selection.Include) == 0 && len(selection.Exclude) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(selection)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (selection Selection) excluded(relative string) bool {
	return selection.matches(selection.Exclude, relative, true)
}

func (selection Selection) includes(relative string) bool {
	if len(selection.Include) == 0 {
		return true
	}
	return selection.matches(selection.Include, relative, true)
}

func (selection Selection) mayContainIncluded(relative string) bool {
	for _, pattern := range selection.Include {
		if pattern == relative || strings.HasPrefix(pattern, relative+"/") || strings.HasPrefix(relative, pattern+"/") {
			return true
		}
		// Wildcards may select descendants of this directory. Keep walking when
		// the fixed prefix of a pattern can still be reached from this path.
		fixedPrefix := strings.SplitN(pattern, "*", 2)[0]
		fixedPrefix = strings.TrimSuffix(fixedPrefix, "/")
		if fixedPrefix == "" || fixedPrefix == relative || strings.HasPrefix(fixedPrefix, relative+"/") || strings.HasPrefix(relative, fixedPrefix+"/") {
			return true
		}
	}
	return false
}

func (selection Selection) matches(patterns []string, relative string, ancestors bool) bool {
	for _, pattern := range patterns {
		for candidate := relative; ; candidate = path.Dir(candidate) {
			matched, err := path.Match(pattern, candidate)
			if err == nil && matched {
				return true
			}
			if !strings.Contains(pattern, "/") {
				matched, err = path.Match(pattern, path.Base(candidate))
				if err == nil && matched {
					return true
				}
			}
			if candidate == "." || !ancestors {
				break
			}
		}
	}
	return false
}
