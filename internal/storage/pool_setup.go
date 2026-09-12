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

// PoolSetupStep describes one phase of a first-run pool setup sequence.
type PoolSetupStep struct {
	Action      string `json:"action"`
	DiskID      string `json:"diskId,omitempty"`
	Filesystem  string `json:"filesystem,omitempty"`
	Description string `json:"description"`
}

// PoolSetupPlan bundles the ordered operations that turn blank disks into a
// mounted mergerfs pool with optional SnapRAID protection: format+mount each
// member at its canonical branch path, mount the pool, apply the protection
// layout, and queue the initial sync. The bundle hash is confirmed once; the
// privileged broker revalidates every disk identity at execution time, so
// identity changes after planning always abort the run.
type PoolSetupPlan struct {
	OperationID      string          `json:"operationId"`
	Name             string          `json:"name"`
	MountPath        string          `json:"mountPath"`
	DataDiskIDs      []string        `json:"dataDiskIds"`
	ParityDiskID     string          `json:"parityDiskId,omitempty"`
	FormatDiskIDs    []string        `json:"formatDiskIds"`
	Filesystem       string          `json:"filesystem"`
	DestroysData     bool            `json:"destroysData"`
	Steps            []PoolSetupStep `json:"steps"`
	ConfigGeneration int64           `json:"configGeneration"`
	ExpiresAt        time.Time       `json:"expiresAt"`
	PlanHash         string          `json:"planHash"`
	Status           string          `json:"status"`
}

// NewPoolSetupPlan validates a first-run pool setup request against the
// current disk inventory and produces the reviewed, hash-sealed sequence.
func NewPoolSetupPlan(id, name string, disks []model.Disk, dataIDs []string, parityID, filesystem string, forceFormat bool, generation int64, now time.Time) (PoolSetupPlan, error) {
	if id == "" {
		return PoolSetupPlan{}, errors.New("operation id is required")
	}
	if !poolNamePattern.MatchString(name) {
		return PoolSetupPlan{}, errors.New("pool name must contain lowercase letters, numbers, and hyphens")
	}
	if filesystem == "" {
		filesystem = "ext4"
	}
	if filesystem != "ext4" && filesystem != "xfs" {
		return PoolSetupPlan{}, fmt.Errorf("filesystem %q is not supported", filesystem)
	}
	if len(dataIDs) == 0 {
		return PoolSetupPlan{}, errors.New("at least one data disk is required")
	}
	byID := make(map[string]model.Disk, len(disks))
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	seenData := make(map[string]bool, len(dataIDs))
	formatIDs := make([]string, 0, len(dataIDs))
	for _, id := range dataIDs {
		disk, ok := byID[id]
		if !ok {
			return PoolSetupPlan{}, fmt.Errorf("data disk %q is not currently discovered", id)
		}
		if seenData[id] {
			return PoolSetupPlan{}, fmt.Errorf("data disk %q is listed more than once", id)
		}
		if parityID == id {
			return PoolSetupPlan{}, errors.New("a disk cannot be both parity and pool data")
		}
		if disk.PoolID != "" {
			return PoolSetupPlan{}, fmt.Errorf("disk %q is already assigned to pool %q", id, disk.PoolID)
		}
		if disk.Role == "parity" {
			return PoolSetupPlan{}, fmt.Errorf("parity disk %q cannot become pool data", id)
		}
		if disk.Health == model.Critical {
			return PoolSetupPlan{}, fmt.Errorf("disk %q is critically unhealthy", id)
		}
		seenData[id] = true
		usable := disk.Filesystem == "ext4" || disk.Filesystem == "xfs"
		if forceFormat || !usable {
			formatIDs = append(formatIDs, id)
		}
	}
	if parityID != "" {
		parity, ok := byID[parityID]
		if !ok {
			return PoolSetupPlan{}, fmt.Errorf("parity disk %q is not currently discovered", parityID)
		}
		if parity.PoolID != "" {
			return PoolSetupPlan{}, fmt.Errorf("parity disk %q is already assigned to pool %q", parityID, parity.PoolID)
		}
	}
	plan := PoolSetupPlan{
		OperationID:      id,
		Name:             name,
		MountPath:        "/srv/pools/" + name,
		DataDiskIDs:      append([]string(nil), dataIDs...),
		ParityDiskID:     parityID,
		FormatDiskIDs:    formatIDs,
		Filesystem:       filesystem,
		DestroysData:     len(formatIDs) > 0,
		Steps:            make([]PoolSetupStep, 0, len(formatIDs)+3),
		ConfigGeneration: generation,
		ExpiresAt:        now.Add(15 * time.Minute),
		Status:           "planned",
	}
	for _, id := range formatIDs {
		plan.Steps = append(plan.Steps, PoolSetupStep{Action: "filesystem.create", DiskID: id, Filesystem: filesystem, Description: "Format " + id + " (" + filesystem + ") and mount it at its branch path — existing data is destroyed"})
	}
	plan.Steps = append(plan.Steps, PoolSetupStep{Action: "pool.mount", Description: "Mount the mergerfs pool at " + plan.MountPath})
	if parityID != "" {
		plan.Steps = append(plan.Steps, PoolSetupStep{Action: "snapraid.apply", Description: "Configure SnapRAID parity on " + parityID + " protecting " + fmt.Sprint(len(dataIDs)) + " data disks"})
		plan.Steps = append(plan.Steps, PoolSetupStep{Action: "snapraid.sync", Description: "Queue the initial SnapRAID sync"})
	}
	plan.PlanHash = HashPoolSetupPlan(plan)
	return plan, nil
}

func HashPoolSetupPlan(plan PoolSetupPlan) string {
	plan.PlanHash = ""
	data, _ := json.Marshal(plan)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ValidatePoolSetupPlan re-checks the bundle against the live inventory right
// before execution: disks present, roles still safe, generation unchanged.
func ValidatePoolSetupPlan(plan PoolSetupPlan, actual []model.Disk, now time.Time, generation int64) error {
	if plan.Status != "planned" && plan.Status != "confirmed" {
		return fmt.Errorf("pool setup status %q cannot be executed", plan.Status)
	}
	if !now.Before(plan.ExpiresAt) {
		return errors.New("pool setup plan has expired")
	}
	if plan.PlanHash == "" || plan.PlanHash != HashPoolSetupPlan(plan) {
		return errors.New("pool setup plan hash mismatch")
	}
	if plan.ConfigGeneration != generation {
		return errors.New("configuration generation changed after planning")
	}
	byID := make(map[string]model.Disk, len(actual))
	for _, disk := range actual {
		byID[disk.ID] = disk
	}
	for _, id := range plan.DataDiskIDs {
		disk, ok := byID[id]
		if !ok {
			return fmt.Errorf("data disk %q is no longer present", id)
		}
		if disk.PoolID != "" {
			return fmt.Errorf("data disk %q became a member of pool %q", id, disk.PoolID)
		}
	}
	if plan.ParityDiskID != "" {
		disk, ok := byID[plan.ParityDiskID]
		if !ok {
			return fmt.Errorf("parity disk %q is no longer present", plan.ParityDiskID)
		}
		if disk.PoolID != "" {
			return fmt.Errorf("parity disk %q became a member of pool %q", plan.ParityDiskID, disk.PoolID)
		}
	}
	return nil
}
