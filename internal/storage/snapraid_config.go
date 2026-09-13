package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lumonas/lumonas/internal/model"
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
	branches := make(map[string]string, len(dataDiskIDs)+1)
	if parityDiskID != "" {
		if !model.HasStableDiskIdentity(parityDiskID) {
			return "", fmt.Errorf("parity disk %q has no stable identity", parityDiskID)
		}
		branches[DiskBranchPath(parityDiskID)] = parityDiskID
	}
	for _, id := range dataDiskIDs {
		if !model.HasStableDiskIdentity(id) {
			return "", fmt.Errorf("data disk %q has no stable identity", id)
		}
		if seen[id] {
			return "", fmt.Errorf("data disk %q is listed more than once", id)
		}
		if parityDiskID != "" && id == parityDiskID {
			return "", fmt.Errorf("parity disk %q cannot also be a protected data disk", parityDiskID)
		}
		branch := DiskBranchPath(id)
		if previous, exists := branches[branch]; exists {
			return "", fmt.Errorf("disk identities %q and %q resolve to the same branch path %q", previous, id, branch)
		}
		branches[branch] = id
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "", fmt.Errorf("at least one protected data disk is required")
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
	dataBranches := make(map[string]bool)
	parityBranch := ""
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
			if !strings.HasSuffix(path, "/"+ParityFileName) {
				return fmt.Errorf("parity path %q is not canonical", path)
			}
			parityBranch = strings.TrimSuffix(path, "/"+ParityFileName)
			if _, ok := managedDiskBranch(parityBranch); !ok {
				return fmt.Errorf("parity path %q is not canonical", path)
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
			if _, ok := managedDiskBranch(path); !ok {
				return fmt.Errorf("data path %q is not canonical", path)
			}
			if dataBranches[path] {
				return fmt.Errorf("data branch path %q is used more than once", path)
			}
			dataBranches[path] = true
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
	if parityBranch != "" && dataBranches[parityBranch] {
		return fmt.Errorf("parity branch path %q is also assigned as a data branch", parityBranch)
	}
	return nil
}

func safeManagedPath(value string) bool {
	clean := filepath.Clean(value)
	if clean == SystemContentDir || strings.HasPrefix(clean, SystemContentDir+"/") {
		return true
	}
	if _, found := managedDiskBranch(clean); found {
		return true
	}
	for _, filename := range []string{ParityFileName, ContentFileName} {
		if strings.HasSuffix(clean, "/"+filename) {
			_, found := managedDiskBranch(strings.TrimSuffix(clean, "/"+filename))
			return found
		}
	}
	return false
}

func managedDiskBranch(value string) (string, bool) {
	clean := filepath.Clean(value)
	if clean != value {
		return "", false
	}
	segment, found := strings.CutPrefix(clean, "/srv/disks/")
	if !found || !validUnitSegment(segment) {
		return "", false
	}
	return clean, true
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
