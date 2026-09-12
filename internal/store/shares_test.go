package store

import (
	"database/sql"
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestManagedShareCRUDAndAccessRuleCleanup(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	group, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindGroup, Name: "family"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := database.CreateManagedShare(shares.ManagedShare{
		ID: "share-documents", Name: "Documents", Path: "/srv/Documents", Enabled: true,
		Protocols: []shares.Protocol{{Name: "smb"}, {Name: "nfs", Settings: map[string]any{"rootSquash": true}}},
		Access:    []shares.AccessRule{{PrincipalID: group.ID, Level: "write"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Protocols) != 2 || len(created.Access) != 1 || created.Access[0].PrincipalName != "family" {
		t.Fatalf("unexpected created share: %#v", created)
	}

	updated := created
	updated.Description = "Team documents"
	updated.Protocols = []shares.Protocol{{Name: "smb"}}
	if _, err := database.UpdateManagedShare(updated); err != nil {
		t.Fatal(err)
	}
	loaded, err := database.ManagedShare(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Description != "Team documents" || len(loaded.Protocols) != 1 {
		t.Fatalf("unexpected updated share: %#v", loaded)
	}
	if err := database.DeleteManagedShare(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ManagedShare(created.ID); err != sql.ErrNoRows {
		t.Fatalf("expected deleted share, got %v", err)
	}
	if err := database.DeletePrincipal(group.ID); err != nil {
		t.Fatalf("share access rule was not cleaned up: %v", err)
	}
}

func TestImportLegacySharesCreatesCanonicalRecords(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	legacyPath := t.TempDir() + "/shares.json"
	legacy := shares.Store{Path: legacyPath}
	if err := legacy.Save([]shares.Share{{ID: "legacy-1", Name: "Media", Path: "/srv/Media", Enabled: true, Protocols: []string{"smb"}, Access: map[string]string{"family": "read"}}}); err != nil {
		t.Fatal(err)
	}
	if err := database.ImportLegacyShares(legacyPath); err != nil {
		t.Fatal(err)
	}
	values, err := database.ListManagedShares()
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].ID != "legacy-1" || len(values[0].Access) != 1 {
		t.Fatalf("unexpected imported shares: %#v", values)
	}
	principal, err := database.PrincipalByName("family")
	if err != nil || principal.Kind != identity.KindGroup {
		t.Fatalf("legacy access principal was not imported as group: %#v err=%v", principal, err)
	}
}
