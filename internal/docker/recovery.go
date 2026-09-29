package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/runner"
)

type RecoveryStrategy string

const (
	StrategyStopBackup RecoveryStrategy = "stop-backup"
	StrategySnapshot   RecoveryStrategy = "snapshot"
	StrategyCustom     RecoveryStrategy = "custom"
	StrategyNone       RecoveryStrategy = "none"
)

type RecoveryHook struct {
	Container string `json:"container,omitempty"`
	Command   string `json:"command"`
	Timeout   int    `json:"timeout,omitempty"`
}

type RecoveryContract struct {
	Strategy       RecoveryStrategy `json:"strategy"`
	AppdataPaths   []string         `json:"appdataPaths,omitempty"`
	PreBackupHook  *RecoveryHook    `json:"preBackupHook,omitempty"`
	PostBackupHook *RecoveryHook    `json:"postBackupHook,omitempty"`
	RestoreHook    *RecoveryHook    `json:"restoreHook,omitempty"`
	DBDump         *RecoveryHook    `json:"dbDump,omitempty"`
	DBRestore      *RecoveryHook    `json:"dbRestore,omitempty"`
	StopServices   []string         `json:"stopServices,omitempty"`
	StartServices  []string         `json:"startServices,omitempty"`
}

func DefaultRecoveryContract() *RecoveryContract {
	return &RecoveryContract{
		Strategy: StrategyStopBackup,
	}
}

func ValidateRecoveryContract(c *RecoveryContract) error {
	if c == nil {
		return nil
	}
	switch c.Strategy {
	case StrategyStopBackup, StrategySnapshot, StrategyCustom, StrategyNone:
	default:
		return fmt.Errorf("invalid recovery strategy: %s", c.Strategy)
	}
	if c.Strategy == StrategyNone {
		return nil
	}
	if c.Strategy == StrategyCustom {
		if c.PreBackupHook == nil && c.PostBackupHook == nil && c.RestoreHook == nil {
			return fmt.Errorf("custom strategy requires at least one hook")
		}
		if len(c.AppdataPaths) > 0 && (c.PreBackupHook == nil || c.PostBackupHook == nil) {
			return fmt.Errorf("custom appdata backup requires both pre-backup and post-backup hooks")
		}
	}
	for _, path := range c.AppdataPaths {
		if strings.Contains(path, "..") {
			return fmt.Errorf("appdata path must not contain ..")
		}
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("appdata path must be absolute: %s", path)
		}
	}
	if c.PreBackupHook != nil {
		if err := validateHook(c.PreBackupHook); err != nil {
			return fmt.Errorf("pre-backup hook: %w", err)
		}
		if c.PreBackupHook.Container == "" {
			return fmt.Errorf("pre-backup hook requires a Compose service")
		}
	}
	if c.PostBackupHook != nil {
		if err := validateHook(c.PostBackupHook); err != nil {
			return fmt.Errorf("post-backup hook: %w", err)
		}
		if c.PostBackupHook.Container == "" {
			return fmt.Errorf("post-backup hook requires a Compose service")
		}
	}
	if c.RestoreHook != nil {
		if err := validateHook(c.RestoreHook); err != nil {
			return fmt.Errorf("restore hook: %w", err)
		}
		if c.RestoreHook.Container == "" {
			return fmt.Errorf("restore hook requires a Compose service")
		}
	}
	if c.DBDump != nil {
		if err := validateHook(c.DBDump); err != nil {
			return fmt.Errorf("database dump hook: %w", err)
		}
		if c.DBDump.Container == "" {
			return fmt.Errorf("database dump hook requires a Compose service")
		}
	}
	if c.DBRestore != nil {
		if err := validateHook(c.DBRestore); err != nil {
			return fmt.Errorf("database restore hook: %w", err)
		}
		if c.DBRestore.Container == "" {
			return fmt.Errorf("database restore hook requires a Compose service")
		}
	}
	return nil
}

func validateHook(h *RecoveryHook) error {
	if h.Command == "" {
		return fmt.Errorf("command is required")
	}
	if len(h.Command) > 4096 || strings.ContainsAny(h.Command, "\x00\r\n") {
		return fmt.Errorf("command is invalid or too long")
	}
	if h.Container != "" && !recoveryContainerPattern.MatchString(h.Container) {
		return fmt.Errorf("container must be a valid Compose service name")
	}
	if h.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative")
	}
	if h.Timeout > 3600 {
		return fmt.Errorf("timeout must not exceed one hour")
	}
	return nil
}

var recoveryContainerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// RunRecoveryHook invokes a catalog- or administrator-authored recovery hook
// in a named Compose service. Output is bounded by the caller because database
// dump commands can legitimately be much larger than ordinary CLI output.
func (s Service) RunRecoveryHook(ctx context.Context, stack string, hook *RecoveryHook, outputLimit int64) ([]byte, error) {
	if !validStackName(stack) {
		return nil, errors.New("invalid stack name")
	}
	if hook == nil {
		return nil, errors.New("recovery hook is required")
	}
	if err := validateHook(hook); err != nil {
		return nil, err
	}
	if hook.Container == "" {
		return nil, errors.New("recovery hook requires a Compose service")
	}
	if outputLimit <= 0 {
		return nil, errors.New("recovery hook output limit must be positive")
	}
	composePath := filepath.Join(s.Root, stack, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		return nil, err
	}
	timeout := time.Duration(hook.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if ctx == nil {
		ctx = context.Background()
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"compose", "-f", composePath, "exec", "-T", hook.Container, "sh", "-c", hook.Command}
	if s.engine == nil && s.Run != nil {
		return s.Run(commandCtx, "docker", args...)
	}
	return runner.OutputContextLimitTimeout(commandCtx, timeout, outputLimit, "docker", args...)
}

func ContractFromCatalog(catalogAppdataPaths []string, catalogDBDumpContainer string) *RecoveryContract {
	contract := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: catalogAppdataPaths,
	}
	if catalogDBDumpContainer != "" {
		contract.DBDump = &RecoveryHook{
			Container: catalogDBDumpContainer,
			Command:   "pg_dump -U postgres",
			Timeout:   300,
		}
		contract.DBRestore = &RecoveryHook{
			Container: catalogDBDumpContainer,
			Command:   "psql -U postgres",
			Timeout:   300,
		}
	}
	return contract
}

func MergeRecoveryContracts(base *RecoveryContract, override *RecoveryContract) *RecoveryContract {
	if override == nil {
		return base
	}
	if base == nil {
		return override
	}
	merged := &RecoveryContract{
		Strategy:      override.Strategy,
		AppdataPaths:  append(base.AppdataPaths, override.AppdataPaths...),
		StopServices:  append(base.StopServices, override.StopServices...),
		StartServices: append(base.StartServices, override.StartServices...),
	}
	if override.PreBackupHook != nil {
		merged.PreBackupHook = override.PreBackupHook
	} else {
		merged.PreBackupHook = base.PreBackupHook
	}
	if override.PostBackupHook != nil {
		merged.PostBackupHook = override.PostBackupHook
	} else {
		merged.PostBackupHook = base.PostBackupHook
	}
	if override.RestoreHook != nil {
		merged.RestoreHook = override.RestoreHook
	} else {
		merged.RestoreHook = base.RestoreHook
	}
	if override.DBDump != nil {
		merged.DBDump = override.DBDump
	} else {
		merged.DBDump = base.DBDump
	}
	if override.DBRestore != nil {
		merged.DBRestore = override.DBRestore
	} else {
		merged.DBRestore = base.DBRestore
	}
	seen := make(map[string]bool)
	unique := merged.AppdataPaths[:0]
	for _, p := range merged.AppdataPaths {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}
	merged.AppdataPaths = unique
	return merged
}

func StackRecoveryCoverage(contract *RecoveryContract) float64 {
	if contract == nil || contract.Strategy == StrategyNone {
		return 0
	}
	score := 0.5
	if len(contract.AppdataPaths) > 0 {
		score += 0.25
	}
	if contract.DBDump != nil {
		score += 0.15
	}
	if contract.RestoreHook != nil {
		score += 0.1
	}
	return score
}
