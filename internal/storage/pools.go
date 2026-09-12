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

var poolNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type PoolMemberPlan struct {
	DiskID         string `json:"diskId"`
	WWN            string `json:"wwn,omitempty"`
	Serial         string `json:"serial,omitempty"`
	Model          string `json:"model,omitempty"`
	SizeBytes      uint64 `json:"sizeBytes"`
	FilesystemUUID string `json:"filesystemUuid,omitempty"`
	BranchPath     string `json:"branchPath"`
}

type PoolPlan struct {
	OperationID      string           `json:"operationId"`
	Name             string           `json:"name"`
	MountPath        string           `json:"mountPath"`
	Policy           string           `json:"policy"`
	Members          []PoolMemberPlan `json:"members"`
	ConfigGeneration int64            `json:"configGeneration"`
	ExpiresAt        time.Time        `json:"expiresAt"`
	PlanHash         string           `json:"planHash"`
	Status           string           `json:"status"`
}

func NewPoolPlan(id, name, mountPath string, disks []model.Disk, generation int64, now time.Time) (PoolPlan, error) {
	if id == "" {
		return PoolPlan{}, errors.New("operation id is required")
	}
	if !poolNamePattern.MatchString(name) {
		return PoolPlan{}, errors.New("pool name must contain lowercase letters, numbers, and hyphens")
	}
	if mountPath != "/srv/pools/"+name {
		return PoolPlan{}, errors.New("pool mount path must be the canonical /srv/pools/<name> path")
	}
	if len(disks) == 0 {
		return PoolPlan{}, errors.New("at least one disk is required")
	}
	plan := PoolPlan{OperationID: id, Name: name, MountPath: mountPath, Policy: "mfs", ConfigGeneration: generation, ExpiresAt: now.Add(15 * time.Minute), Status: "planned", Members: make([]PoolMemberPlan, 0, len(disks))}
	seen := make(map[string]bool, len(disks))
	for _, disk := range disks {
		if disk.ID == "" || disk.CurrentPath == "" {
			return PoolPlan{}, errors.New("every pool disk requires a stable identity and current path")
		}
		if seen[disk.ID] {
			return PoolPlan{}, fmt.Errorf("disk %q is listed more than once", disk.ID)
		}
		if disk.PoolID != "" {
			return PoolPlan{}, fmt.Errorf("disk %q is already assigned to pool %q", disk.ID, disk.PoolID)
		}
		if disk.Role == "parity" {
			return PoolPlan{}, fmt.Errorf("parity disk %q cannot be added to a mergerfs pool", disk.ID)
		}
		if disk.Filesystem != "" && disk.Filesystem != "ext4" && disk.Filesystem != "xfs" {
			return PoolPlan{}, fmt.Errorf("disk %q uses unsupported filesystem %q", disk.ID, disk.Filesystem)
		}
		seen[disk.ID] = true
		plan.Members = append(plan.Members, PoolMemberPlan{DiskID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, FilesystemUUID: disk.FilesystemUUID, BranchPath: DiskBranchPath(disk.ID)})
	}
	plan.PlanHash = HashPoolPlan(plan)
	return plan, nil
}

func HashPoolPlan(plan PoolPlan) string {
	plan.PlanHash = ""
	data, _ := json.Marshal(plan)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func ValidatePoolPlan(plan PoolPlan, actual []model.Disk, now time.Time, generation int64) error {
	if plan.Status != "planned" && plan.Status != "confirmed" {
		return fmt.Errorf("pool plan status %q cannot be executed", plan.Status)
	}
	if !now.Before(plan.ExpiresAt) {
		return errors.New("pool plan has expired")
	}
	if plan.PlanHash == "" || plan.PlanHash != HashPoolPlan(plan) {
		return errors.New("pool plan hash mismatch")
	}
	if plan.ConfigGeneration != generation {
		return errors.New("configuration generation changed after planning")
	}
	if !poolNamePattern.MatchString(plan.Name) || plan.MountPath != "/srv/pools/"+plan.Name {
		return errors.New("pool plan path is invalid")
	}
	if plan.Policy != "mfs" {
		return fmt.Errorf("pool policy %q is not allow-listed", plan.Policy)
	}
	byID := make(map[string]model.Disk, len(actual))
	for _, disk := range actual {
		byID[disk.ID] = disk
	}
	if len(plan.Members) == 0 {
		return errors.New("pool plan has no members")
	}
	seen := make(map[string]bool, len(plan.Members))
	for _, member := range plan.Members {
		if seen[member.DiskID] {
			return fmt.Errorf("pool plan contains duplicate disk %q", member.DiskID)
		}
		seen[member.DiskID] = true
		disk, ok := byID[member.DiskID]
		if !ok {
			return fmt.Errorf("pool disk %q is no longer present", member.DiskID)
		}
		if member.WWN != "" && disk.WWN != member.WWN {
			return fmt.Errorf("pool disk %q WWN mismatch", member.DiskID)
		}
		if member.Serial != "" && disk.Serial != member.Serial {
			return fmt.Errorf("pool disk %q serial mismatch", member.DiskID)
		}
		if member.Model != "" && disk.Model != member.Model {
			return fmt.Errorf("pool disk %q model mismatch", member.DiskID)
		}
		if member.SizeBytes != 0 && disk.SizeBytes != member.SizeBytes {
			return fmt.Errorf("pool disk %q capacity mismatch", member.DiskID)
		}
		if member.FilesystemUUID != "" && disk.FilesystemUUID != member.FilesystemUUID {
			return fmt.Errorf("pool disk %q filesystem UUID mismatch", member.DiskID)
		}
		if disk.PoolID != "" {
			return fmt.Errorf("pool disk %q is already assigned to pool %q", member.DiskID, disk.PoolID)
		}
		if disk.Health == model.Critical {
			return fmt.Errorf("pool disk %q is critically unhealthy", member.DiskID)
		}
		if member.BranchPath != DiskBranchPath(member.DiskID) {
			return fmt.Errorf("pool disk %q branch path mismatch", member.DiskID)
		}
	}
	return nil
}

func DiskBranchPath(diskID string) string {
	var builder strings.Builder
	for _, value := range diskID {
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9') || value == '.' || value == '_' || value == '-' {
			builder.WriteRune(value)
		} else {
			builder.WriteByte('_')
		}
	}
	segment := builder.String()
	if segment == "" {
		segment = "unknown"
	}
	return "/srv/disks/" + segment
}

func MergerFSCommand(plan PoolPlan) ([]string, error) {
	if plan.PlanHash == "" || plan.PlanHash != HashPoolPlan(plan) {
		return nil, errors.New("pool plan hash mismatch")
	}
	if !poolNamePattern.MatchString(plan.Name) || plan.MountPath != "/srv/pools/"+plan.Name {
		return nil, errors.New("pool plan path is invalid")
	}
	if plan.Policy != "mfs" {
		return nil, fmt.Errorf("pool policy %q is not allow-listed", plan.Policy)
	}
	branches := make([]string, 0, len(plan.Members))
	for _, member := range plan.Members {
		if member.BranchPath != DiskBranchPath(member.DiskID) {
			return nil, errors.New("pool branch path is not canonical")
		}
		branches = append(branches, member.BranchPath)
	}
	if len(branches) == 0 {
		return nil, errors.New("pool plan has no branches")
	}
	sort.Strings(branches)
	return []string{"-t", "fuse.mergerfs", "-o", "defaults,allow_other,use_ino,category.create=mfs", strings.Join(branches, ":"), plan.MountPath}, nil
}
