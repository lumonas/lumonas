package docker

import (
	"fmt"
	"strings"
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
	}
	if c.PostBackupHook != nil {
		if err := validateHook(c.PostBackupHook); err != nil {
			return fmt.Errorf("post-backup hook: %w", err)
		}
	}
	if c.RestoreHook != nil {
		if err := validateHook(c.RestoreHook); err != nil {
			return fmt.Errorf("restore hook: %w", err)
		}
	}
	return nil
}

func validateHook(h *RecoveryHook) error {
	if h.Command == "" {
		return fmt.Errorf("command is required")
	}
	if h.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative")
	}
	return nil
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
