package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestShareAccessPreviewShowsEffectiveGroupAndGuestGrants(t *testing.T) {
	server := testServer(t)
	user, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "family", Password: "long-family-password", ManagementRole: identity.RoleNone})
	if err != nil {
		t.Fatal(err)
	}
	group, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindGroup, Name: "readers", ManagementRole: identity.RoleNone})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetGroupMembers(group.ID, []string{user.ID}); err != nil {
		t.Fatal(err)
	}
	share, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "preview-share", Name: "Media", Path: "/srv/media", Enabled: true, Guest: true, Protocols: []shares.Protocol{{Name: "smb"}}, Access: []shares.AccessRule{{PrincipalID: group.ID, Level: "read"}}})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/shares/"+share.ID+"/access-preview", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("access preview status %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Entries []shareAccessPreviewEntry `json:"entries"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var member, guest bool
	for _, entry := range result.Entries {
		if entry.PrincipalID == user.ID {
			member = entry.Level == identity.AccessRead && len(entry.GrantedBy) == 1 && entry.GrantedBy[0] == "group:readers"
		}
		if entry.Guest {
			guest = entry.Level == identity.AccessRead
		}
	}
	if !member || !guest {
		t.Fatalf("preview missed an effective grant: %#v", result.Entries)
	}
}
