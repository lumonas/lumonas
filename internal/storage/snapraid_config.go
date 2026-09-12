package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// SnapRAID configuration paths are canonical: parity and content files live
// at the root of the canonical disk branch, and one content copy lives on
// the system disk next to the LumoNAS database.
const (
	ParityFileName   = "snapraid.parity"
	ContentFileName  = "snapraid.content"
	SystemContentDir = "/var/lib/lumonas"
)

// RenderSnapraidConfig renders a complete snapraid.conf from stable disk
// identities. Data names d1..dN are assigned in sorted disk-ID order so the
// generated configuration is stable across reboots and device reordering.
// Up to two content copies are placed on the first data disks in addition
// to the system copy, per the content replication policy.
func RenderSnapraidConfig(parityDiskID string, dataDiskIDs []string) (string, error) {
	ids := make([]string, 0, len(dataDiskIDs))
	seen := make(map[string]bool, len(dataDiskIDs))
	for _, id := range dataDiskIDs {
		if id == "" {
			return "", fmt.Errorf("data disk identity is empty")
		}
		if seen[id] {
			return "", fmt.Errorf("data disk %q is listed more than once", id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "", fmt.Errorf("at least one protected data disk is required")
	}
	if parityDiskID != "" {
		if seen[parityDiskID] {
			return "", fmt.Errorf("parity disk %q cannot also be a protected data disk", parityDiskID)
		}
	}
	var builder strings.Builder
	builder.WriteString("# " + UnitDirectoryHint + "\n")
	if parityDiskID != "" {
		fmt.Fprintf(&builder, "parity %s/%s\n", DiskBranchPath(parityDiskID), ParityFileName)
	}
	fmt.Fprintf(&builder, "content %s/%s\n", SystemContentDir, ContentFileName)
	contentCopies := 2
	if len(ids) < contentCopies {
		contentCopies = len(ids)
	}
	for _, id := range ids[:contentCopies] {
		fmt.Fprintf(&builder, "content %s/%s\n", DiskBranchPath(id), ContentFileName)
	}
	for index, id := range ids {
		fmt.Fprintf(&builder, "data d%d %s\n", index+1, DiskBranchPath(id))
	}
	return builder.String(), nil
}

// ValidateSnapraidConfig performs structural validation of a rendered
// SnapRAID configuration before it is activated: only managed directives,
// canonical paths, unique data names, and non-overlapping assignments are
// accepted.
func ValidateSnapraidConfig(content string) error {
	parity := 0
	contentCopies := 0
	dataNames := make(map[string]bool)
	usedPaths := make(map[string]bool)
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return fmt.Errorf("configuration line %q is invalid", line)
		}
		path := fields[len(fields)-1]
		if !safeManagedPath(path) {
			return fmt.Errorf("configuration path %q is outside the managed locations", path)
		}
		if usedPaths[path] {
			return fmt.Errorf("configuration path %q is used more than once", path)
		}
		usedPaths[path] = true
		switch fields[0] {
		case "parity":
			parity++
			if len(fields) != 2 {
				return fmt.Errorf("parity line %q is invalid", line)
			}
		case "content":
			contentCopies++
			if len(fields) != 2 {
				return fmt.Errorf("content line %q is invalid", line)
			}
		case "data":
			if len(fields) != 3 {
				return fmt.Errorf("data line %q is invalid", line)
			}
			if !validSnapraidName(fields[1]) {
				return fmt.Errorf("data name %q is invalid", fields[1])
			}
			if dataNames[fields[1]] {
				return fmt.Errorf("data name %q is duplicated", fields[1])
			}
			dataNames[fields[1]] = true
		default:
			return fmt.Errorf("unsupported configuration directive %q", fields[0])
		}
	}
	if parity > 1 {
		return fmt.Errorf("at most one parity file is supported, found %d", parity)
	}
	if len(dataNames) == 0 {
		return errors.New("at least one data disk is required")
	}
	if contentCopies < 2 {
		return errors.New("at least two content copies are required")
	}
	return nil
}

func safeManagedPath(value string) bool {
	clean := filepath.Clean(value)
	if clean == SystemContentDir || strings.HasPrefix(clean, SystemContentDir+"/") {
		return true
	}
	segment, found := strings.CutPrefix(clean, "/srv/disks/")
	return found && segment != "" && !strings.Contains(segment, "..")
}

func validSnapraidName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}
