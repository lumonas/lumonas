package docker

import (
	"testing"
)

func TestValidateRecoveryContractNil(t *testing.T) {
	if err := ValidateRecoveryContract(nil); err != nil {
		t.Fatalf("nil contract should be valid: %v", err)
	}
}

func TestValidateRecoveryContractInvalidStrategy(t *testing.T) {
	c := &RecoveryContract{Strategy: "bad"}
	if err := ValidateRecoveryContract(c); err == nil {
		t.Fatal("invalid strategy should fail")
	}
}

func TestValidateRecoveryContractNoneStrategy(t *testing.T) {
	c := &RecoveryContract{Strategy: StrategyNone}
	if err := ValidateRecoveryContract(c); err != nil {
		t.Fatalf("none strategy should be valid: %v", err)
	}
}

func TestValidateRecoveryContractCustomRequiresHooks(t *testing.T) {
	c := &RecoveryContract{Strategy: StrategyCustom}
	if err := ValidateRecoveryContract(c); err == nil {
		t.Fatal("custom strategy without hooks should fail")
	}
	c.PreBackupHook = &RecoveryHook{Command: "echo pre"}
	if err := ValidateRecoveryContract(c); err != nil {
		t.Fatalf("custom strategy with pre-backup hook should be valid: %v", err)
	}
}

func TestValidateRecoveryContractRejectsRelativePath(t *testing.T) {
	c := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: []string{"../etc/passwd"},
	}
	if err := ValidateRecoveryContract(c); err == nil {
		t.Fatal("relative path should fail")
	}
}

func TestValidateRecoveryContractRejectsNonAbsolutePath(t *testing.T) {
	c := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: []string{"var/data"},
	}
	if err := ValidateRecoveryContract(c); err == nil {
		t.Fatal("non-absolute path should fail")
	}
}

func TestValidateRecoveryContractAcceptsAbsolutePaths(t *testing.T) {
	c := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: []string{"/var/lib/appdata", "/opt/app/config"},
	}
	if err := ValidateRecoveryContract(c); err != nil {
		t.Fatalf("valid paths rejected: %v", err)
	}
}

func TestValidateRecoveryContractRejectsEmptyHookCommand(t *testing.T) {
	c := &RecoveryContract{
		Strategy:      StrategyStopBackup,
		PreBackupHook: &RecoveryHook{},
	}
	if err := ValidateRecoveryContract(c); err == nil {
		t.Fatal("empty hook command should fail")
	}
}

func TestValidateRecoveryContractRejectsNegativeTimeout(t *testing.T) {
	c := &RecoveryContract{
		Strategy:      StrategyStopBackup,
		PreBackupHook: &RecoveryHook{Command: "echo", Timeout: -1},
	}
	if err := ValidateRecoveryContract(c); err == nil {
		t.Fatal("negative timeout should fail")
	}
}

func TestDefaultRecoveryContract(t *testing.T) {
	c := DefaultRecoveryContract()
	if c == nil {
		t.Fatal("default contract should not be nil")
	}
	if c.Strategy != StrategyStopBackup {
		t.Fatalf("expected stop-backup strategy, got %s", c.Strategy)
	}
}

func TestContractFromCatalog(t *testing.T) {
	c := ContractFromCatalog([]string{"/var/lib/appdata"}, "db-container")
	if c.Strategy != StrategyStopBackup {
		t.Fatalf("expected stop-backup strategy, got %s", c.Strategy)
	}
	if len(c.AppdataPaths) != 1 || c.AppdataPaths[0] != "/var/lib/appdata" {
		t.Fatalf("unexpected appdata paths: %v", c.AppdataPaths)
	}
	if c.DBDump == nil || c.DBDump.Container != "db-container" {
		t.Fatal("expected DBDump hook with db-container")
	}
	if c.DBRestore == nil || c.DBRestore.Container != "db-container" {
		t.Fatal("expected DBRestore hook with db-container")
	}
}

func TestContractFromCatalogNoDB(t *testing.T) {
	c := ContractFromCatalog([]string{"/data"}, "")
	if c.DBDump != nil {
		t.Fatal("no DB dump expected without container")
	}
}

func TestMergeRecoveryContracts(t *testing.T) {
	base := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: []string{"/data1"},
	}
	override := &RecoveryContract{
		Strategy:      StrategySnapshot,
		AppdataPaths:  []string{"/data2"},
		PreBackupHook: &RecoveryHook{Command: "echo pre"},
	}
	merged := MergeRecoveryContracts(base, override)
	if merged.Strategy != StrategySnapshot {
		t.Fatalf("expected override strategy, got %s", merged.Strategy)
	}
	if len(merged.AppdataPaths) != 2 {
		t.Fatalf("expected 2 appdata paths, got %d", len(merged.AppdataPaths))
	}
	if merged.PreBackupHook == nil || merged.PreBackupHook.Command != "echo pre" {
		t.Fatal("expected override pre-backup hook")
	}
}

func TestMergeRecoveryContractsNilOverride(t *testing.T) {
	base := &RecoveryContract{Strategy: StrategyStopBackup}
	merged := MergeRecoveryContracts(base, nil)
	if merged != base {
		t.Fatal("nil override should return base")
	}
}

func TestMergeRecoveryContractsNilBase(t *testing.T) {
	override := &RecoveryContract{Strategy: StrategySnapshot}
	merged := MergeRecoveryContracts(nil, override)
	if merged != override {
		t.Fatal("nil base should return override")
	}
}

func TestMergeRecoveryContractsDeduplicatesPaths(t *testing.T) {
	base := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: []string{"/data", "/shared"},
	}
	override := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: []string{"/shared", "/other"},
	}
	merged := MergeRecoveryContracts(base, override)
	if len(merged.AppdataPaths) != 3 {
		t.Fatalf("expected 3 unique paths, got %d: %v", len(merged.AppdataPaths), merged.AppdataPaths)
	}
}

func TestStackRecoveryCoverageNil(t *testing.T) {
	if score := StackRecoveryCoverage(nil); score != 0 {
		t.Fatalf("expected 0 for nil, got %f", score)
	}
}

func TestStackRecoveryCoverageNone(t *testing.T) {
	c := &RecoveryContract{Strategy: StrategyNone}
	if score := StackRecoveryCoverage(c); score != 0 {
		t.Fatalf("expected 0 for none strategy, got %f", score)
	}
}

func TestStackRecoveryCoverageMinimal(t *testing.T) {
	c := &RecoveryContract{Strategy: StrategyStopBackup}
	if score := StackRecoveryCoverage(c); score != 0.5 {
		t.Fatalf("expected 0.5 for minimal contract, got %f", score)
	}
}

func TestStackRecoveryCoverageFull(t *testing.T) {
	c := &RecoveryContract{
		Strategy:     StrategyStopBackup,
		AppdataPaths: []string{"/data"},
		DBDump:       &RecoveryHook{Command: "pg_dump"},
		RestoreHook:  &RecoveryHook{Command: "restore"},
	}
	if score := StackRecoveryCoverage(c); score != 1.0 {
		t.Fatalf("expected 1.0 for full contract, got %f", score)
	}
}
