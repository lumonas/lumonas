package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/store"
)

func TestAPITokenScopeGate(t *testing.T) {
	server := testServer(t)
	server.authRequired, server.dynamicAuth = true, false
	owner, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "owner", Password: "owner-password", ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := server.store.CreateAPIToken(store.APITokenCreate{ID: "token-read", OwnerID: owner.ID, Name: "monitor", Scopes: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil)
	read.Header.Set("Authorization", "Bearer "+raw)
	readResult := httptest.NewRecorder()
	server.routes().ServeHTTP(readResult, read)
	if readResult.Code != http.StatusOK {
		t.Fatalf("read token was rejected: %d %s", readResult.Code, readResult.Body.String())
	}
	write := httptest.NewRequest(http.MethodPost, "/api/v1/backups/run", nil)
	write.Header.Set("Authorization", "Bearer "+raw)
	writeResult := httptest.NewRecorder()
	server.routes().ServeHTTP(writeResult, write)
	if writeResult.Code != http.StatusForbidden {
		t.Fatalf("read token performed write: %d %s", writeResult.Code, writeResult.Body.String())
	}
}

func TestFleetStatusTokenIsNarrowlyScoped(t *testing.T) {
	server := testServer(t)
	server.authRequired, server.dynamicAuth, server.version = true, false, "test-build"
	owner, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "fleet-owner", Password: "owner-password", ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := server.store.CreateAPIToken(store.APITokenCreate{ID: "token-fleet", OwnerID: owner.ID, Name: "peer status", Scopes: []string{"fleet:status"}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/fleet/status", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"version":"test-build"`) {
		t.Fatalf("fleet status scope failed: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/server", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("fleet status token read unrestricted server data: %d %s", response.Code, response.Body.String())
	}
}

func TestSnapshotReplicationReceiveTokenIsBoundToOneShare(t *testing.T) {
	scopes := []string{"replication:snapshot:receive:share-target"}
	if !validTokenScopes(scopes) || !apiTokenAllows(scopes, http.MethodPut, "/api/v1/replication/snapshots/share-target/receive") || !apiTokenAllows(scopes, http.MethodPost, "/api/v1/replication/snapshots/share-target/cancel") {
		t.Fatal("valid share-bound snapshot receive scope was rejected")
	}
	if apiTokenAllows(scopes, http.MethodPut, "/api/v1/replication/snapshots/share-other/receive") || apiTokenAllows(scopes, http.MethodGet, "/api/v1/replication/snapshot-tasks") || validTokenScopes(append(scopes, "read")) || validTokenScopes([]string{"replication:snapshot:receive:bad/share"}) {
		t.Fatal("snapshot receive scope escaped its destination or combined with broader access")
	}
	if snapshotReplicationReceiveShare(scopes) != "share-target" {
		t.Fatal("destination share could not be extracted from token scope")
	}
}

func TestWorkstationBackupTokenIsShareBound(t *testing.T) {
	server := testServer(t)
	server.authRequired, server.dynamicAuth = true, false
	owner, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "backup-owner", Password: "owner-password", ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := server.store.CreateAPIToken(store.APITokenCreate{ID: "token-workstation", OwnerID: owner.ID, Name: "laptop", Scopes: []string{"workstation:backup:share-photos"}})
	if err != nil {
		t.Fatal(err)
	}
	if !validTokenScopes([]string{"workstation:backup:share-photos"}) || validTokenScopes([]string{"workstation:backup:bad/share"}) {
		t.Fatal("workstation share token validation failed")
	}
	workstationScopes := []string{"workstation:backup:share-photos"}
	if !apiTokenAllows(workstationScopes, http.MethodPost, "/api/v1/files/uploads") || !apiTokenAllows(workstationScopes, http.MethodGet, "/api/v1/files/download") || !apiTokenAllows(workstationScopes, http.MethodGet, "/api/v1/files") || apiTokenAllows(workstationScopes, http.MethodPost, "/api/v1/files/delete") || apiTokenAllows(workstationScopes, http.MethodGet, "/api/v1/server") {
		t.Fatal("workstation token has an unexpected API surface")
	}
	if validTokenScopes([]string{"workstation:backup:share-photos", "read"}) || apiTokenAllows([]string{"workstation:backup:share-photos", "read"}, http.MethodGet, "/api/v1/server") {
		t.Fatal("workstation backup scope can be combined with broad access")
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/files/download?share=share-other&path=/&name=backup.lwb", nil)
	request.Header.Set("Authorization", "Bearer "+raw)
	if server.workstationShareAllowed(request, "share-other") || !server.workstationShareAllowed(request, "share-photos") {
		t.Fatal("workstation token was not constrained to its selected share")
	}
}

func TestWorkstationBackupTokenUploadsOnlyToBoundShare(t *testing.T) {
	server := testServer(t)
	server.authRequired, server.dynamicAuth = true, false
	owner, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "backup-client-owner", Password: "owner-password", ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	firstRoot, secondRoot := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	for _, root := range []string{firstRoot, secondRoot} {
		if err := os.Mkdir(root, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, managed := range []shares.ManagedShare{{ID: "share-backup-one", Name: "One", Path: firstRoot, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}, {ID: "share-backup-two", Name: "Two", Path: secondRoot, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}} {
		if _, err := server.store.CreateManagedShare(managed); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LUMONAS_UPLOAD_SESSION_DIR", filepath.Join(t.TempDir(), "sessions"))
	_, raw, err := server.store.CreateAPIToken(store.APITokenCreate{ID: "token-workstation-route", OwnerID: owner.ID, Name: "single share client", Scopes: []string{"workstation:backup:share-backup-one"}})
	if err != nil {
		t.Fatal(err)
	}
	post := func(shareID string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"shareId":%q,"path":"/","name":"archive.lwb","sizeBytes":4}`, shareID)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/files/uploads", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+raw)
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, request)
		return response
	}
	denied := post("share-backup-two")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("token wrote to an unbound share: %d %s", denied.Code, denied.Body.String())
	}
	allowed := post("share-backup-one")
	if allowed.Code != http.StatusCreated {
		t.Fatalf("token could not create upload in its selected share: %d %s", allowed.Code, allowed.Body.String())
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(allowed.Body.Bytes(), &session); err != nil || session.ID == "" {
		t.Fatalf("invalid upload session response: %#v err=%v", session, err)
	}
	download := httptest.NewRequest(http.MethodGet, "/api/v1/files/download?share=share-backup-two&path=/&name=archive.lwb", nil)
	download.Header.Set("Authorization", "Bearer "+raw)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, download)
	if response.Code != http.StatusForbidden {
		t.Fatalf("token downloaded from an unbound share: %d %s", response.Code, response.Body.String())
	}
	wrongListing := httptest.NewRequest(http.MethodGet, "/api/v1/files?share=share-backup-two&path=/", nil)
	wrongListing.Header.Set("Authorization", "Bearer "+raw)
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, wrongListing)
	if response.Code != http.StatusForbidden {
		t.Fatalf("token listed an unbound share: %d %s", response.Code, response.Body.String())
	}
	listing := httptest.NewRequest(http.MethodGet, "/api/v1/files?share=share-backup-one&path=/", nil)
	listing.Header.Set("Authorization", "Bearer "+raw)
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, listing)
	if response.Code != http.StatusOK {
		t.Fatalf("token could not list archives in its selected share: %d %s", response.Code, response.Body.String())
	}
}
