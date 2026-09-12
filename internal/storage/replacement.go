package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

// DataSlot pins a SnapRAID data name to a disk identity. Replacement flows
// must keep the retired disk's name pointing at the replacement so
// `snapraid fix -d <name>` recovers the lost content from parity.
type DataSlot struct {
	Name   string `json:"name"`
	DiskID string `json:"diskId"`
}

var dataNamePattern = regexp.MustCompile(`^d[0-9]+$`)

// ParseSnapraidDataMapping extracts the pinned name→disk mapping from a
// managed SnapRAID configuration. Parity is returned separately.
func ParseSnapraidDataMapping(content string) (parityDiskID string, slots []DataSlot, err error) {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) == 2 && fields[0] == "parity":
			// The parity line points at the parity file inside the parity
			// disk's branch; the identity is the directory segment.
			filePath := fields[1]
			separator := strings.LastIndexByte(filePath, '/')
			if separator <= len("/srv/disks/") {
				return "", nil, fmt.Errorf("parity path %q is not a canonical branch path", filePath)
			}
			id, parseErr := DiskIDFromBranchPath(filePath[:separator])
			if parseErr != nil {
				return "", nil, parseErr
			}
			if parityDiskID != "" {
				return "", nil, errors.New("multiple parity lines in managed config")
			}
			parityDiskID = id
		case len(fields) == 3 && fields[0] == "data":
			if !dataNamePattern.MatchString(fields[1]) {
				return "", nil, fmt.Errorf("unmanaged data name %q", fields[1])
			}
			id, parseErr := DiskIDFromBranchPath(fields[2])
			if parseErr != nil {
				return "", nil, parseErr
			}
			slots = append(slots, DataSlot{Name: fields[1], DiskID: id})
		}
	}
	if len(slots) == 0 {
		return "", nil, errors.New("managed config contains no data disks")
	}
	return parityDiskID, slots, nil
}

// DiskIDFromBranchPath converts /srv/disks/<id> back to <id>.
func DiskIDFromBranchPath(branchPath string) (string, error) {
	const prefix = "/srv/disks/"
	if !strings.HasPrefix(branchPath, prefix) || len(branchPath) <= len(prefix) {
		return "", fmt.Errorf("path %q is not a canonical branch path", branchPath)
	}
	return strings.TrimPrefix(branchPath, prefix), nil
}

// RenderSnapraidConfigPinned renders a managed config from an explicit
// name→disk mapping, keeping every surviving disk on its historical name.
func RenderSnapraidConfigPinned(parityDiskID string, slots []DataSlot) (string, error) {
	if len(slots) == 0 {
		return "", errors.New("at least one protected data disk is required")
	}
	names := make(map[string]bool, len(slots))
	paths := make(map[string]bool, len(slots))
	for _, slot := range slots {
		if !dataNamePattern.MatchString(slot.Name) {
			return "", fmt.Errorf("unmanaged data name %q", slot.Name)
		}
		if names[slot.Name] {
			return "", fmt.Errorf("data name %q is duplicated", slot.Name)
		}
		if slot.DiskID == "" {
			return "", errors.New("data disk identity is empty")
		}
		branch := DiskBranchPath(slot.DiskID)
		if paths[branch] {
			return "", fmt.Errorf("disk %q is assigned twice", slot.DiskID)
		}
		if parityDiskID != "" && slot.DiskID == parityDiskID {
			return "", fmt.Errorf("parity disk %q cannot also be a protected data disk", parityDiskID)
		}
		names[slot.Name] = true
		paths[branch] = true
	}
	ordered := append([]DataSlot(nil), slots...)
	sort.Slice(ordered, func(i, j int) bool {
		if numericSuffix(ordered[i].Name) == numericSuffix(ordered[j].Name) {
			return ordered[i].Name < ordered[j].Name
		}
		return numericSuffix(ordered[i].Name) < numericSuffix(ordered[j].Name)
	})
	var builder strings.Builder
	builder.WriteString("# " + UnitDirectoryHint + "\n")
	if parityDiskID != "" {
		fmt.Fprintf(&builder, "parity %s/%s\n", DiskBranchPath(parityDiskID), ParityFileName)
	}
	fmt.Fprintf(&builder, "content %s/%s\n", SystemContentDir, ContentFileName)
	contentCopies := 2
	if len(ordered) < contentCopies {
		contentCopies = len(ordered)
	}
	for _, slot := range ordered[:contentCopies] {
		fmt.Fprintf(&builder, "content %s/%s\n", DiskBranchPath(slot.DiskID), ContentFileName)
	}
	for _, slot := range ordered {
		fmt.Fprintf(&builder, "data %s %s\n", slot.Name, DiskBranchPath(slot.DiskID))
	}
	return builder.String(), nil
}

func numericSuffix(name string) int {
	value := 0
	for _, char := range name {
		if char >= '0' && char <= '9' {
			value = value*10 + int(char-'0')
		}
	}
	return value
}

// ReplacementPlan is the reviewed sequence for swapping a failed data disk:
// format the replacement, re-point the retired slot at it, recover the lost
// content from parity (snapraid fix), and re-sync.
type ReplacementPlan struct {
	OperationID       string            `json:"operationId"`
	RetiredDiskID     string            `json:"retiredDiskId"`
	RetiredDataName   string            `json:"retiredDataName"`
	ReplacementDiskID string            `json:"replacementDiskId"`
	ParityDiskID      string            `json:"parityDiskId,omitempty"`
	Slots             []DataSlot        `json:"slots"`
	ExpectedDisk      PoolSetupIdentity `json:"expectedDisk"`
	ConfigGeneration  int64             `json:"configGeneration"`
	ExpiresAt         time.Time         `json:"expiresAt"`
	PlanHash          string            `json:"planHash"`
	Status            string            `json:"status"`
}

// NewReplacementPlan builds the plan from the current managed mapping plus
// the live inventory. The replacement takes over the retired disk's data
// name so parity recovery targets the right slot.
func NewReplacementPlan(id, retiredDiskID, replacementDiskID string, currentConfig string, disks []model.Disk, generation int64, now time.Time) (ReplacementPlan, error) {
	if id == "" {
		return ReplacementPlan{}, errors.New("operation id is required")
	}
	parity, slots, err := ParseSnapraidDataMapping(currentConfig)
	if err != nil {
		return ReplacementPlan{}, fmt.Errorf("current SnapRAID configuration is not readable as managed config: %w", err)
	}
	byID := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	retiredName := ""
	found := false
	retiredBranch := DiskBranchPath(retiredDiskID)
	for _, slot := range slots {
		if "/srv/disks/"+slot.DiskID == retiredBranch {
			retiredName = slot.Name
			found = true
			break
		}
	}
	if !found {
		return ReplacementPlan{}, fmt.Errorf("retired disk %q is not part of the protected set", retiredDiskID)
	}
	replacement, ok := byID[replacementDiskID]
	if !ok {
		return ReplacementPlan{}, errors.New("replacement disk is not currently discovered")
	}
	if replacement.PoolID != "" {
		return ReplacementPlan{}, fmt.Errorf("replacement disk %q is already assigned to pool %q", replacementDiskID, replacement.PoolID)
	}
	if replacement.Role == "parity" {
		return ReplacementPlan{}, errors.New("a parity-role disk cannot be the replacement data disk")
	}
	if replacement.Mounted {
		return ReplacementPlan{}, fmt.Errorf("replacement disk %q is already mounted", replacementDiskID)
	}
	if replacement.Health == model.Critical {
		return ReplacementPlan{}, fmt.Errorf("replacement disk %q is critically unhealthy", replacementDiskID)
	}
	for index, slot := range slots {
		branch := "/srv/disks/" + slot.DiskID
		if branch == retiredBranch {
			slots[index].DiskID = replacementDiskID
			continue
		}
		mappedID := ""
		for _, disk := range disks {
			if DiskBranchPath(disk.ID) == branch {
				mappedID = disk.ID
				break
			}
		}
		if mappedID == "" {
			return ReplacementPlan{}, fmt.Errorf("protected disk branch %q is no longer discovered", branch)
		}
		slots[index].DiskID = mappedID
	}
	if retiredName == "" {
		return ReplacementPlan{}, errors.New("retired slot has no data name")
	}
	if parity == replacementDiskID {
		return ReplacementPlan{}, errors.New("replacement disk cannot be the parity disk")
	}
	plan := ReplacementPlan{
		OperationID:       id,
		RetiredDiskID:     retiredDiskID,
		RetiredDataName:   retiredName,
		ReplacementDiskID: replacementDiskID,
		ParityDiskID:      parity,
		Slots:             slots,
		ExpectedDisk:      poolSetupIdentity(replacement),
		ConfigGeneration:  generation,
		ExpiresAt:         now.Add(15 * time.Minute),
		Status:            "planned",
	}
	plan.PlanHash = HashReplacementPlan(plan)
	return plan, nil
}

func HashReplacementPlan(plan ReplacementPlan) string {
	plan.PlanHash = ""
	data, _ := json.Marshal(plan)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ValidateReplacementPlan re-checks the plan before execution.
func ValidateReplacementPlan(plan ReplacementPlan, actual []model.Disk, now time.Time, generation int64) error {
	if plan.Status != "planned" && plan.Status != "confirmed" {
		return fmt.Errorf("replacement status %q cannot be executed", plan.Status)
	}
	if !now.Before(plan.ExpiresAt) {
		return errors.New("replacement plan has expired")
	}
	if plan.PlanHash == "" || plan.PlanHash != HashReplacementPlan(plan) {
		return errors.New("replacement plan hash mismatch")
	}
	if plan.ConfigGeneration != generation {
		return errors.New("configuration generation changed after planning")
	}
	for _, disk := range actual {
		if disk.ID != plan.ReplacementDiskID {
			continue
		}
		if disk.PoolID != "" {
			return fmt.Errorf("replacement disk became a member of pool %q", disk.PoolID)
		}
		if disk.Mounted {
			return errors.New("replacement disk became mounted before confirmation")
		}
		if disk.WWN != plan.ExpectedDisk.WWN || disk.Serial != plan.ExpectedDisk.Serial || disk.Model != plan.ExpectedDisk.Model || disk.SizeBytes != plan.ExpectedDisk.SizeBytes || disk.GPTDiskGUID != plan.ExpectedDisk.GPTDiskGUID || disk.PartitionUUID != plan.ExpectedDisk.PartitionUUID {
			return errors.New("replacement disk identity changed")
		}
		if disk.Health == model.Critical {
			return errors.New("replacement disk became critically unhealthy")
		}
		return nil
	}
	return fmt.Errorf("replacement disk %q is no longer present", plan.ReplacementDiskID)
}
