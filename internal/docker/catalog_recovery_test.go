package docker

import "testing"

func TestEnrichStackMatchesCatalogAndBuildsRecoveryContract(t *testing.T) {
	stack := Stack{ID: "stack-jellyfin", Name: "jellyfin", Images: []string{"jellyfin/jellyfin:10.10.6"}}
	apps := []CatalogApp{{
		ID:           "jellyfin",
		Category:     "Media",
		Image:        "jellyfin/jellyfin:10.10.6",
		AppdataPaths: []string{"/config"},
		Recovery:     &RecoveryContract{Strategy: StrategyStopBackup},
	}}

	enriched := EnrichStack(stack, apps)
	if enriched.CatalogID != "jellyfin" || enriched.Category != "Media" {
		t.Fatalf("catalog metadata was not applied: %#v", enriched)
	}
	if enriched.Recovery == nil || len(enriched.Recovery.AppdataPaths) != 1 || enriched.Recovery.AppdataPaths[0] != "/config" {
		t.Fatalf("recovery contract was not derived: %#v", enriched.Recovery)
	}
	if enriched.RecoveryCoverage <= 0 {
		t.Fatalf("expected non-zero recovery coverage: %f", enriched.RecoveryCoverage)
	}
}

func TestEnrichStackUsesConservativeContractForImportedStack(t *testing.T) {
	stack := EnrichStack(Stack{ID: "stack-custom", Name: "custom", Images: []string{"example/custom:latest"}}, nil)
	if stack.CatalogID != "" || stack.Recovery == nil {
		t.Fatalf("expected conservative imported-stack metadata: %#v", stack)
	}
	if stack.Recovery.Strategy != StrategyStopBackup || stack.RecoveryCoverage != 0.5 {
		t.Fatalf("unexpected imported-stack contract: %#v coverage=%f", stack.Recovery, stack.RecoveryCoverage)
	}
}
