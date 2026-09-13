package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

// minSystemDiskBytes is a floor, not a recommendation: a root filesystem
// below this size cannot hold the appliance packages and state.
const minSystemDiskBytes = 8 << 30

// PlanTTL bounds how long a generated install plan stays applicable.
const PlanTTL = 10 * time.Minute

var hostnamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var adminNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
var filesystems = map[string]bool{"ext4": true, "xfs": true}

// Target is one disk considered as an installation destination. Protected
// disks stay listed with their reasons so the review step can show why a
// disk is excluded instead of hiding it.
type Target struct {
	DiskID      string            `json:"diskId"`
	Name        string            `json:"name"`
	Model       string            `json:"model"`
	SizeBytes   uint64            `json:"sizeBytes"`
	Role        string            `json:"role"`
	Filesystem  string            `json:"filesystem,omitempty"`
	Mounted     bool              `json:"mounted"`
	Eligible    bool              `json:"eligible"`
	ProtectedBy []string          `json:"protectedBy,omitempty"`
	Identity    map[string]string `json:"identity"`
}

// Plan is the immutable description of one installation. Apply requests must
// quote its exact hash; the privileged worker revalidates the target identity
// before touching any bytes.
type Plan struct {
	ID               string            `json:"id"`
	TargetDiskID     string            `json:"targetDiskId"`
	ExpectedIdentity map[string]string `json:"expectedIdentity"`
	Hostname         string            `json:"hostname"`
	AdminUsername    string            `json:"adminUsername"`
	Filesystem       string            `json:"filesystem"`
	UEFI             bool              `json:"uefi"`
	CreatedAt        time.Time         `json:"createdAt"`
	ExpiresAt        time.Time         `json:"expiresAt"`
}

// EvaluateTargets reviews collector output for installation suitability.
// The disk the live system runs from, mounted data disks, parity members,
// and disks carrying filesystems are reported as protected with reasons.
func EvaluateTargets(disks []model.Disk, systemDiskID string) []Target {
	targets := make([]Target, 0, len(disks))
	for _, disk := range disks {
		target := Target{
			DiskID:     disk.ID,
			Name:       disk.Name,
			Model:      disk.Model,
			SizeBytes:  disk.SizeBytes,
			Role:       disk.Role,
			Filesystem: disk.Filesystem,
			Mounted:    disk.Mounted,
			Identity:   diskIdentity(disk),
		}
		if disk.ID == systemDiskID {
			target.ProtectedBy = append(target.ProtectedBy, "running system disk")
		}
		if disk.Role == "parity" {
			target.ProtectedBy = append(target.ProtectedBy, "SnapRAID parity disk")
		}
		if disk.Mounted {
			target.ProtectedBy = append(target.ProtectedBy, "disk is mounted")
		}
		if disk.Filesystem != "" {
			target.ProtectedBy = append(target.ProtectedBy, "disk holds an existing filesystem")
		}
		if disk.SizeBytes < minSystemDiskBytes {
			target.ProtectedBy = append(target.ProtectedBy, fmt.Sprintf("smaller than the %d GiB minimum", minSystemDiskBytes>>30))
		}
		if disk.Health == model.Critical {
			target.ProtectedBy = append(target.ProtectedBy, "disk health is critical")
		}
		target.Eligible = len(target.ProtectedBy) == 0
		targets = append(targets, target)
	}
	return targets
}

func diskIdentity(disk model.Disk) map[string]string {
	identity := map[string]string{
		"sizeBytes": strconv.FormatUint(disk.SizeBytes, 10),
	}
	if disk.WWN != "" {
		identity["wwn"] = disk.WWN
	}
	if disk.Serial != "" {
		identity["serial"] = disk.Serial
	}
	if disk.GPTDiskGUID != "" {
		identity["gptDiskGuid"] = disk.GPTDiskGUID
	}
	return identity
}

// BuildRequest carries the operator's installation decisions.
type BuildRequest struct {
	TargetDiskID  string
	Hostname      string
	AdminUsername string
	Filesystem    string
	UEFI          bool
}

// BuildPlan validates the request against the reviewed targets and produces
// the immutable plan.
func BuildPlan(disks []model.Disk, systemDiskID string, request BuildRequest, now time.Time) (Plan, []Target, error) {
	if !hostnamePattern.MatchString(strings.TrimSpace(request.Hostname)) {
		return Plan{}, nil, errors.New("hostname must contain lowercase letters, numbers, and hyphens")
	}
	if !adminNamePattern.MatchString(strings.TrimSpace(request.AdminUsername)) {
		return Plan{}, nil, errors.New("administrator name is invalid")
	}
	if !filesystems[request.Filesystem] {
		return Plan{}, nil, errors.New("filesystem must be ext4 or xfs")
	}
	targets := EvaluateTargets(disks, systemDiskID)
	var chosen *Target
	for index := range targets {
		if targets[index].DiskID == request.TargetDiskID {
			chosen = &targets[index]
			break
		}
	}
	if chosen == nil {
		return Plan{}, targets, errors.New("target disk is not present")
	}
	if !chosen.Eligible {
		return Plan{}, targets, fmt.Errorf("target disk is protected: %s", strings.Join(chosen.ProtectedBy, "; "))
	}
	if len(chosen.Identity) < 2 {
		return Plan{}, targets, errors.New("target disk lacks a stable identity for revalidation")
	}
	plan := Plan{
		ID:               "install-" + strconv.FormatInt(now.UTC().UnixNano(), 10),
		TargetDiskID:     chosen.DiskID,
		ExpectedIdentity: chosen.Identity,
		Hostname:         strings.TrimSpace(request.Hostname),
		AdminUsername:    strings.TrimSpace(request.AdminUsername),
		Filesystem:       request.Filesystem,
		UEFI:             request.UEFI,
		CreatedAt:        now.UTC(),
		ExpiresAt:        now.UTC().Add(PlanTTL),
	}
	return plan, targets, nil
}

// Hash pins the plan content; apply requests must quote this value.
func (p Plan) Hash() string {
	encoded, err := json.Marshal(struct {
		TargetDiskID     string            `json:"targetDiskId"`
		ExpectedIdentity map[string]string `json:"expectedIdentity"`
		Hostname         string            `json:"hostname"`
		AdminUsername    string            `json:"adminUsername"`
		Filesystem       string            `json:"filesystem"`
		UEFI             bool              `json:"uefi"`
		CreatedAt        time.Time         `json:"createdAt"`
	}{
		TargetDiskID:     p.TargetDiskID,
		ExpectedIdentity: p.ExpectedIdentity,
		Hostname:         p.Hostname,
		AdminUsername:    p.AdminUsername,
		Filesystem:       p.Filesystem,
		UEFI:             p.UEFI,
		CreatedAt:        p.CreatedAt,
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// ValidateForApply enforces the confirmation contract: the quoted hash must
// match, the plan must be unexpired, and apply must be explicitly confirmed.
func (p Plan) ValidateForApply(hash string, confirm bool, now time.Time) error {
	if !confirm {
		return errors.New("installation must be explicitly confirmed")
	}
	if hash == "" || hash != p.Hash() {
		return errors.New("plan hash does not match the generated installation plan")
	}
	if now.After(p.ExpiresAt) {
		return errors.New("installation plan has expired")
	}
	return nil
}
