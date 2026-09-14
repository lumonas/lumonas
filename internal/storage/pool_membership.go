package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

// PoolMembershipPlan grows an existing mergerfs pool by adding member disks:
// format and mount each new disk at its branch path, unmount the pool,
// re-mount it over the full branch list, and extend the SnapRAID layout with
// a fresh data slot. Shrink (draining data off a member) is deliberately out
// of scope: mergerfs spreads files across branches, so removing a member can
// only be safe after an explicit data drain that does not exist yet.
type PoolMembershipPlan struct {
	OperationID      string    `json:"operationId"`
	PoolName         string    `json:"poolName"`
	MountPath        string    `json:"mountPath"`
	ExistingBranches []string  `json:"existingBranches"`
	AddDiskIDs       []string  `json:"addDiskIds"`
	FormatDiskIDs    []string  `json:"formatDiskIds"`
	Filesystem       string    `json:"filesystem"`
	NewBranches      []string  `json:"newBranches"`
	ConfigGeneration int64     `json:"configGeneration"`
	ExpiresAt        time.Time `json:"expiresAt"`
	PlanHash         string    `json:"planHash"`
	Status           string    `json:"status"`
}

// NewPoolMembershipPlan validates a grow request against the mounted pool
// and the live inventory.
func NewPoolMembershipPlan(id string, pool model.Pool, disks []model.Disk, addIDs []string, filesystem string, forceFormat bool, generation int64, now time.Time) (PoolMembershipPlan, error) {
	if id == "" {
		return PoolMembershipPlan{}, errors.New("operation id is required")
	}
	if !poolNamePattern.MatchString(pool.Name) || pool.MountPath != "/srv/pools/"+pool.Name {
		return PoolMembershipPlan{}, errors.New("pool mount path is invalid")
	}
	if filesystem == "" {
		filesystem = "ext4"
	}
	if filesystem != "ext4" && filesystem != "xfs" {
		return PoolMembershipPlan{}, fmt.Errorf("filesystem %q is not supported", filesystem)
	}
	if len(addIDs) == 0 {
		return PoolMembershipPlan{}, errors.New("at least one disk is required to grow the pool")
	}
	if len(pool.Members) == 0 {
		return PoolMembershipPlan{}, errors.New("pool has no existing members")
	}
	existing := make([]string, 0, len(pool.Members))
	branches := make(map[string]bool)
	for _, member := range pool.Members {
		if member.BranchPath == "" {
			return PoolMembershipPlan{}, errors.New("pool member is missing its branch path")
		}
		existing = append(existing, member.BranchPath)
		branches[member.BranchPath] = true
	}
	byID := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	plan := PoolMembershipPlan{
		OperationID:      id,
		PoolName:         pool.Name,
		MountPath:        pool.MountPath,
		ExistingBranches: existing,
		AddDiskIDs:       append([]string(nil), addIDs...),
		Filesystem:       filesystem,
		FormatDiskIDs:    []string{},
		ConfigGeneration: generation,
		ExpiresAt:        now.Add(15 * time.Minute),
		Status:           "planned",
	}
	newBranches := append([]string(nil), existing...)
	for _, memberID := range plan.AddDiskIDs {
		disk, ok := byID[memberID]
		if !ok {
			return PoolMembershipPlan{}, fmt.Errorf("disk %q is not currently discovered", memberID)
		}
		if disk.PoolID != "" {
			return PoolMembershipPlan{}, fmt.Errorf("disk %q is already assigned to pool %q", memberID, disk.PoolID)
		}
		if disk.Role == "parity" {
			return PoolMembershipPlan{}, fmt.Errorf("parity disk %q cannot join a mergerfs pool", memberID)
		}
		if disk.Health == model.Critical {
			return PoolMembershipPlan{}, fmt.Errorf("disk %q is critically unhealthy", memberID)
		}
		branch := DiskBranchPath(memberID)
		if branches[branch] {
			return PoolMembershipPlan{}, fmt.Errorf("disk %q is already a member of the pool", memberID)
		}
		branches[branch] = true
		newBranches = append(newBranches, branch)
		if !forceFormat && (disk.Filesystem == "ext4" || disk.Filesystem == "xfs") {
			continue
		}
		plan.FormatDiskIDs = append(plan.FormatDiskIDs, memberID)
	}
	plan.NewBranches = newBranches
	plan.PlanHash = HashPoolMembershipPlan(plan)
	return plan, nil
}

func HashPoolMembershipPlan(plan PoolMembershipPlan) string {
	plan.PlanHash = ""
	data, _ := json.Marshal(plan)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ValidatePoolMembershipPlan re-checks the plan before execution: the pool
// must still be mounted over the same branches and the new disks present.
func ValidatePoolMembershipPlan(plan PoolMembershipPlan, pools []model.Pool, actual []model.Disk, now time.Time, generation int64) error {
	if plan.Status != "planned" && plan.Status != "confirmed" {
		return fmt.Errorf("pool membership status %q cannot be executed", plan.Status)
	}
	if !now.Before(plan.ExpiresAt) {
		return errors.New("pool membership plan has expired")
	}
	if plan.PlanHash == "" || plan.PlanHash != HashPoolMembershipPlan(plan) {
		return errors.New("pool membership plan hash mismatch")
	}
	if plan.ConfigGeneration != generation {
		return errors.New("configuration generation changed after planning")
	}
	currentBranches := make(map[string]bool)
	found := false
	for _, pool := range pools {
		if pool.MountPath != plan.MountPath {
			continue
		}
		found = true
		for _, member := range pool.Members {
			currentBranches[member.BranchPath] = true
		}
		break
	}
	if !found {
		return fmt.Errorf("pool %q is no longer mounted", plan.PoolName)
	}
	for _, branch := range plan.ExistingBranches {
		if !currentBranches[branch] {
			return fmt.Errorf("pool branch %q is no longer mounted", branch)
		}
	}
	for _, branch := range plan.NewBranches {
		if currentBranches[branch] && !containsString(plan.ExistingBranches, branch) {
			return fmt.Errorf("new branch %q is already mounted in the pool", branch)
		}
	}
	byID := make(map[string]model.Disk, len(actual))
	for _, disk := range actual {
		byID[disk.ID] = disk
	}
	for _, id := range plan.AddDiskIDs {
		disk, ok := byID[id]
		if !ok {
			return fmt.Errorf("disk %q is no longer present", id)
		}
		if disk.PoolID != "" {
			return fmt.Errorf("disk %q became a member of pool %q", id, disk.PoolID)
		}
	}
	return nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
