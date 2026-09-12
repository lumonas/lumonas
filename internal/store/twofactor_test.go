package store

import (
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
)

func TestTOTPRecordLifecycleAndSingleUseRecoveryCode(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	principal, err := database.CreatePrincipal(identity.CreateInput{
		Kind:           identity.KindUser,
		Name:           "security-admin",
		Password:       "a-long-development-password",
		ManagementRole: identity.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveTOTPPending(principal.ID, []byte("pending-secret"), []string{"hash-a", "hash-b"}); err != nil {
		t.Fatal(err)
	}
	pending, secret, enabled, hashes, err := database.TOTPRecord(principal.ID)
	if err != nil || string(pending) != "pending-secret" || secret != nil || enabled || len(hashes) != 2 {
		t.Fatalf("unexpected pending TOTP state: pending=%q secret=%q enabled=%v hashes=%v err=%v", pending, secret, enabled, hashes, err)
	}
	if err := database.EnableTOTP(principal.ID, []byte("active-secret")); err != nil {
		t.Fatal(err)
	}
	if enabled, err := database.TOTPEnabled(principal.ID); err != nil || !enabled {
		t.Fatalf("TOTP was not enabled: %v", err)
	}
	consumed, err := database.ConsumeRecoveryCode(principal.ID, "hash-a")
	if err != nil || !consumed {
		t.Fatalf("recovery code was not consumed: %v", err)
	}
	consumed, err = database.ConsumeRecoveryCode(principal.ID, "hash-a")
	if err != nil || consumed {
		t.Fatalf("recovery code was reusable: consumed=%v err=%v", consumed, err)
	}
	if err := database.DisableTOTP(principal.ID); err != nil {
		t.Fatal(err)
	}
	if enabled, err := database.TOTPEnabled(principal.ID); err != nil || enabled {
		t.Fatalf("TOTP was not disabled: %v", err)
	}
}
