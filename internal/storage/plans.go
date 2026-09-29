package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

type Action string

const (
	ActionFormat  Action = "filesystem.format"
	ActionCreate  Action = "filesystem.create"
	ActionErase   Action = "disk.erase"
	ActionMount   Action = "filesystem.mount"
	ActionUnmount Action = "filesystem.unmount"
)

type Plan struct {
	OperationID        string         `json:"operationId"`
	Action             Action         `json:"action"`
	Target             TargetIdentity `json:"target"`
	ExpectedState      ExpectedState  `json:"expectedState"`
	RequestedState     map[string]any `json:"requestedState"`
	DependencySnapshot []string       `json:"dependencySnapshot"`
	ConfigGeneration   int64          `json:"configGeneration"`
	ExpiresAt          time.Time      `json:"expiresAt"`
	PlanHash           string         `json:"planHash"`
	Status             string         `json:"status"`
}

type TargetIdentity struct {
	DiskID         string `json:"diskId"`
	WWN            string `json:"wwn,omitempty"`
	Serial         string `json:"serial,omitempty"`
	Model          string `json:"model,omitempty"`
	SizeBytes      uint64 `json:"sizeBytes"`
	GPTDiskGUID    string `json:"gptDiskGuid,omitempty"`
	PartitionUUID  string `json:"partitionUuid,omitempty"`
	FilesystemUUID string `json:"filesystemUuid,omitempty"`
}

type ExpectedState struct {
	CurrentPath string `json:"currentPath,omitempty"`
	Mounted     bool   `json:"mounted"`
	Role        string `json:"role"`
	PoolID      string `json:"poolId,omitempty"`
}

func NewPlan(id string, action Action, disk model.Disk, generation int64, now time.Time) (Plan, error) {
	if id == "" {
		return Plan{}, errors.New("operation id is required")
	}
	if !model.HasStableDiskIdentity(disk.ID) {
		return Plan{}, errors.New("disk has no stable identity")
	}
	if !supported(action) {
		return Plan{}, fmt.Errorf("unsupported storage action %q", action)
	}
	plan := Plan{
		OperationID: id, Action: action,
		Target:         TargetIdentity{DiskID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, GPTDiskGUID: disk.GPTDiskGUID, PartitionUUID: disk.PartitionUUID, FilesystemUUID: disk.FilesystemUUID},
		ExpectedState:  ExpectedState{CurrentPath: disk.CurrentPath, Mounted: disk.Mounted, Role: disk.Role, PoolID: disk.PoolID},
		RequestedState: map[string]any{}, DependencySnapshot: []string{}, ConfigGeneration: generation, ExpiresAt: now.Add(15 * time.Minute), Status: "planned",
	}
	plan.PlanHash = Hash(plan)
	return plan, nil
}

func Hash(plan Plan) string {
	plan.PlanHash = ""
	bytes, _ := json.Marshal(plan)
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:])
}

func Validate(plan Plan, actual model.Disk, now time.Time, currentGeneration int64) error {
	if plan.Status != "planned" && plan.Status != "confirmed" {
		return fmt.Errorf("plan status %q cannot be executed", plan.Status)
	}
	if !now.Before(plan.ExpiresAt) {
		return errors.New("operation plan has expired")
	}
	if plan.PlanHash == "" || plan.PlanHash != Hash(plan) {
		return errors.New("operation plan hash mismatch")
	}
	if plan.ConfigGeneration != currentGeneration {
		return errors.New("configuration generation changed after planning")
	}
	if !model.HasStableDiskIdentity(plan.Target.DiskID) {
		return errors.New("operation plan targets an unstable disk identity")
	}
	if actual.ID != plan.Target.DiskID {
		return errors.New("stable disk identity mismatch")
	}
	if plan.Target.WWN != "" && actual.WWN != plan.Target.WWN {
		return errors.New("disk WWN mismatch")
	}
	if plan.Target.Serial != "" && actual.Serial != plan.Target.Serial {
		return errors.New("disk serial mismatch")
	}
	if plan.Target.Model != "" && actual.Model != plan.Target.Model {
		return errors.New("disk model mismatch")
	}
	if plan.Target.SizeBytes != 0 && actual.SizeBytes != plan.Target.SizeBytes {
		return errors.New("disk capacity mismatch")
	}
	if plan.Target.GPTDiskGUID != "" && actual.GPTDiskGUID != plan.Target.GPTDiskGUID {
		return errors.New("GPT disk GUID mismatch")
	}
	if plan.Target.PartitionUUID != "" && actual.PartitionUUID != plan.Target.PartitionUUID {
		return errors.New("partition UUID mismatch")
	}
	if plan.Target.FilesystemUUID != "" && actual.FilesystemUUID != plan.Target.FilesystemUUID {
		return errors.New("filesystem UUID mismatch")
	}
	if (plan.Action == ActionFormat || plan.Action == ActionCreate || plan.Action == ActionErase) && actual.Mounted {
		return errors.New("target is currently mounted")
	}
	if (plan.Action == ActionFormat || plan.Action == ActionCreate || plan.Action == ActionErase) && actual.PoolID != "" {
		return errors.New("target is currently assigned to a pool")
	}
	if plan.Action == ActionCreate {
		if err := ValidateRequestedState(plan.Action, plan.Target.DiskID, plan.RequestedState); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRequestedState checks every filesystem operation before a plan is
// shown to an administrator. The privileged worker repeats these checks at
// execution time, but invalid state must never appear as a confirmable plan.
func ValidateRequestedState(action Action, diskID string, requested map[string]any) error {
	if requested == nil {
		requested = map[string]any{}
	}
	filesystem, _ := requested["filesystem"].(string)
	validateFilesystem := func(required bool) error {
		if filesystem == "" && !required {
			return nil
		}
		if filesystem != "ext4" && filesystem != "xfs" {
			return errors.New("filesystem must be ext4 or xfs")
		}
		return nil
	}
	validateMountPath := func(required bool) error {
		mountPath, _ := requested["mountPath"].(string)
		if mountPath == "" && !required {
			return nil
		}
		if filepath.Clean(mountPath) != DiskBranchPath(diskID) {
			return errors.New("mountPath must be the canonical /srv/disks/<disk-id> branch path")
		}
		return nil
	}

	switch action {
	case ActionCreate:
		if _, exists := requested["encryptionPassphrase"]; exists {
			return errors.New("encryptionPassphrase is accepted only during operation confirmation")
		}
		if err := validateFilesystem(true); err != nil {
			return err
		}
		if err := validateMountPath(true); err != nil {
			return err
		}
		if label, _ := requested["label"].(string); label != "" && !ValidFilesystemLabel(label) {
			return errors.New("filesystem label is invalid: must be 1-12 alphanumeric, dot, underscore, or hyphen characters")
		}
		if _, exists := requested["encrypted"]; exists {
			if _, valid := requested["encrypted"].(bool); !valid {
				return errors.New("encrypted must be a boolean")
			}
		}
	case ActionFormat:
		if _, exists := requested["encrypted"]; exists {
			return errors.New("disk encryption is supported only for format-and-mount operations")
		}
		return validateFilesystem(true)
	case ActionMount:
		if err := validateFilesystem(true); err != nil {
			return err
		}
		return validateMountPath(true)
	case ActionUnmount:
		return validateMountPath(true)
	case ActionErase:
		if len(requested) != 0 {
			return errors.New("disk.erase does not accept requested state")
		}
	}
	return nil
}

// ValidFilesystemLabel enforces a label that fits both ext4 (16 chars) and
// xfs (12 chars) limits, starting with an alphanumeric character.
func ValidFilesystemLabel(value string) bool {
	if value == "" || len(value) > 12 {
		return false
	}
	for index, char := range value {
		alphanumeric := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
		if index == 0 && !alphanumeric {
			return false
		}
		if !alphanumeric && char != '.' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func supported(action Action) bool {
	switch action {
	case ActionFormat, ActionCreate, ActionErase, ActionMount, ActionUnmount:
		return true
	default:
		return false
	}
}
