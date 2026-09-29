package store

import (
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
)

func TestAPITokensAreDigestOnlyAndRevocable(t *testing.T) {
	database := testStore(t)
	owner, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "owner", Password: "owner-password", ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	summary, raw, err := database.CreateAPIToken(APITokenCreate{ID: "token-1", OwnerID: owner.ID, Name: "automation", Scopes: []string{"read"}})
	if err != nil || raw == "" || summary.ID != "token-1" {
		t.Fatalf("unexpected token create: %#v raw=%q err=%v", summary, raw, err)
	}
	values, err := database.ListAPITokens(owner.ID)
	if err != nil || len(values) != 1 || values[0].Name != "automation" {
		t.Fatalf("unexpected token inventory %#v err=%v", values, err)
	}
	if values[0].ID == raw {
		t.Fatal("raw API token appeared in inventory")
	}
	name, scopes, ok := database.ResolveAPIToken(raw)
	if !ok || name != "owner" || len(scopes) != 1 || scopes[0] != "read" {
		t.Fatalf("token did not resolve safely: %q %#v %v", name, scopes, ok)
	}
	if err := database.DeleteAPIToken(owner.ID, summary.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := database.ResolveAPIToken(raw); ok {
		t.Fatal("revoked API token remained valid")
	}
}
