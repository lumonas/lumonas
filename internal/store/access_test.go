package store

import (
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestResolveShareAccessIncludesGroupMembership(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "media"})
	if err != nil {
		t.Fatal(err)
	}
	group, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindGroup, Name: "family"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetGroupMembers(group.ID, []string{user.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateManagedShare(shares.ManagedShare{ID: "share-1", Name: "Media", Path: "/srv/media", Protocols: []shares.Protocol{{Name: "smb"}}, Access: []shares.AccessRule{{PrincipalID: group.ID, Level: "write"}}}); err != nil {
		t.Fatal(err)
	}
	level, err := database.ResolveShareAccess("share-1", user.ID)
	if err != nil || level != identity.AccessWrite {
		t.Fatalf("expected inherited write access, got %q err=%v", level, err)
	}
	if !CanAccess(level, identity.AccessRead) || CanAccess(identity.AccessRead, identity.AccessWrite) {
		t.Fatal("access ordering is incorrect")
	}
}
