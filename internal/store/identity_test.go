package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/identity"
)

func TestPrincipalStoreAllocatesStableIDsAndSeparatesManagementAccess(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	fileUser, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "media"})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "operator", Password: "a-long-development-password", ManagementRole: identity.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	if fileUser.UID == nil || admin.UID == nil || *admin.UID <= *fileUser.UID {
		t.Fatalf("expected increasing stable UIDs: %#v %#v", fileUser, admin)
	}
	if _, _, err := database.CreateSession("media", "a-long-development-password", 0); err == nil {
		t.Fatal("file user should not have management login access")
	}
	if _, _, err := database.CreateSession("operator", "a-long-development-password", 0); err != nil {
		t.Fatalf("management user should have login access: %v", err)
	}
}

func TestPrincipalStoreGroupMembershipAndDependencyProtection(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	member, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "family"})
	if err != nil {
		t.Fatal(err)
	}
	group, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindGroup, Name: "household"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetGroupMembers(group.ID, []string{member.ID}); err != nil {
		t.Fatal(err)
	}
	members, err := database.GroupMembers(group.ID)
	if err != nil || len(members) != 1 || members[0].ID != member.ID {
		t.Fatalf("unexpected group members %#v err=%v", members, err)
	}
	if err := database.DeletePrincipal(member.ID); err == nil {
		t.Fatal("group member should not be deleted while referenced")
	}
}

func TestLegacyAdminIsImportedAndDisablingManagementUserRevokesSession(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	password := "a-long-development-password"
	if err := database.EnsureAdmin("admin", password); err != nil {
		t.Fatal(err)
	}
	admin, err := database.PrincipalByName("admin")
	if err != nil || admin.ManagementRole != identity.RoleOwner {
		t.Fatalf("legacy admin was not imported: %#v err=%v", admin, err)
	}
	adminSession, _, err := database.CreateSession("admin", password, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := database.UpdatePrincipal(admin.ID, identity.UpdateInput{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, ok := database.SessionUser(adminSession); ok {
		t.Fatal("disabled management user session remained valid")
	}
	enabled := true
	if _, err := database.UpdatePrincipal(admin.ID, identity.UpdateInput{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.CreateSession("admin", password, time.Hour); err != nil {
		t.Fatalf("re-enabled management user could not authenticate: %v", err)
	}
}

func TestEnsureAdminSynchronizesPrincipalBeforeTwoFactorLookup(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.EnsureAdmin("admin", "a-long-development-password"); err != nil {
		t.Fatal(err)
	}
	userID, err := database.VerifyCredentials("admin", "a-long-development-password")
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := database.TOTPEnabled(userID)
	if err != nil {
		t.Fatalf("immediate two-factor lookup failed: %v", err)
	}
	if enabled {
		t.Fatal("newly provisioned administrator unexpectedly has two-factor enabled")
	}
}
