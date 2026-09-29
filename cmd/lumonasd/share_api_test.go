package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestManagedShareAPICreateUpdateDelete(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	sambaConfig := t.TempDir() + "/generated/smb.conf"
	t.Setenv("LUMONAS_SAMBA_CONFIG", sambaConfig)

	create := httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Documents","path":"/srv/Documents","enabled":true,"protocols":["smb"],"access":{}}`))
	create.Header.Set("Idempotency-Key", "documents-create")
	createdResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(createdResponse, create)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil || created.ID == "" {
		t.Fatalf("invalid create response: %#v err=%v", created, err)
	}
	retry := httptest.NewRecorder()
	server.routes().ServeHTTP(retry, httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Documents","path":"/srv/Documents","enabled":true,"protocols":["smb"],"access":{}}`)))
	// A retry without the key is a normal duplicate request and remains rejected.
	if retry.Code != http.StatusConflict {
		t.Fatalf("unexpected unkeyed duplicate status %d: %s", retry.Code, retry.Body.String())
	}
	keyedRetry := httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Documents","path":"/srv/Documents","enabled":true,"protocols":["smb"],"access":{}}`))
	keyedRetry.Header.Set("Idempotency-Key", "documents-create")
	keyedResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(keyedResponse, keyedRetry)
	if keyedResponse.Code != http.StatusOK || !strings.Contains(keyedResponse.Body.String(), created.ID) {
		t.Fatalf("idempotent retry did not return original share: %d %s", keyedResponse.Code, keyedResponse.Body.String())
	}

	update := httptest.NewRequest(http.MethodPatch, "/api/v1/shares/"+created.ID, strings.NewReader(`{"name":"Documents","path":"/srv/Documents","description":"Updated","enabled":true,"protocols":[{"name":"smb"}],"access":[]}`))
	updatedResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(updatedResponse, update)
	if updatedResponse.Code != http.StatusOK || !strings.Contains(updatedResponse.Body.String(), "Updated") {
		t.Fatalf("update status %d: %s", updatedResponse.Code, updatedResponse.Body.String())
	}

	remove := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/"+created.ID, nil)
	removedResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(removedResponse, remove)
	if removedResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status %d: %s", removedResponse.Code, removedResponse.Body.String())
	}
	if _, err := os.Stat(sambaConfig); !os.IsNotExist(err) {
		t.Fatalf("stale Samba configuration remains: %v", err)
	}
}

func TestManagedShareAPIGeneratesNonSMBConfigurations(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	directory := t.TempDir()
	t.Setenv("LUMONAS_NFS_EXPORTS", directory+"/exports")
	t.Setenv("LUMONAS_SFTP_CONFIG", directory+"/sftp.conf")
	t.Setenv("LUMONAS_FTP_CONFIG", directory+"/ftp.conf")
	t.Setenv("LUMONAS_RSYNC_CONFIG", directory+"/rsync.conf")

	request := httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Media","path":"/srv/media","enabled":true,"protocols":[{"name":"nfs","settings":{"allowedNetworks":["192.168.1.0/24"]}},{"name":"sftp"},{"name":"ftps","settings":{"passivePortStart":40000,"passivePortEnd":40100}},{"name":"rsync","settings":{"readOnly":true}}],"access":[]}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", response.Code, response.Body.String())
	}
	for _, name := range []string{"exports", "sftp.conf", "ftp.conf", "rsync.conf"} {
		if _, err := os.Stat(directory + "/" + name); err != nil {
			t.Fatalf("expected generated %s: %v", name, err)
		}
	}
}

func TestManagedShareSMBAuditCanBeEnabledAndRendered(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	sambaConfig := t.TempDir() + "/generated/smb.conf"
	t.Setenv("LUMONAS_SAMBA_CONFIG", sambaConfig)
	created := httptest.NewRecorder()
	server.routes().ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Records","path":"/srv/records","enabled":true,"protocols":["smb"],"access":{}}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", created.Code, created.Body.String())
	}
	var share struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&share); err != nil || share.ID == "" {
		t.Fatalf("invalid create response %#v err=%v", share, err)
	}
	updated := httptest.NewRecorder()
	server.routes().ServeHTTP(updated, httptest.NewRequest(http.MethodPatch, "/api/v1/shares/"+share.ID+"/protocols/smb", strings.NewReader(`{"auditEnabled":true,"auditOperations":["renameat","unlinkat"]}`)))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"auditEnabled":true`) || !strings.Contains(updated.Body.String(), `"auditOperations":["renameat","unlinkat"]`) {
		t.Fatalf("SMB audit settings were not returned: %d %s", updated.Code, updated.Body.String())
	}
	config, err := os.ReadFile(sambaConfig)
	if err != nil || !strings.Contains(string(config), "vfs objects = shadow_copy2 full_audit") || !strings.Contains(string(config), "full_audit:success = renameat unlinkat") {
		t.Fatalf("SMB audit config was not rendered: %s err=%v", config, err)
	}
	unsafe := httptest.NewRecorder()
	server.routes().ServeHTTP(unsafe, httptest.NewRequest(http.MethodPatch, "/api/v1/shares/"+share.ID+"/protocols/smb", strings.NewReader(`{"auditOperations":["renameat\n   include = /etc/passwd"]}`)))
	if unsafe.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe SMB audit operation accepted: %d %s", unsafe.Code, unsafe.Body.String())
	}
}

func TestModernShareAPIContract(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	t.Setenv("LUMONAS_SAMBA_CONFIG", t.TempDir()+"/generated/smb.conf")

	user := httptest.NewRecorder()
	server.routes().ServeHTTP(user, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"family","password":"a-file-user-password"}`)))
	if user.Code != http.StatusCreated {
		t.Fatalf("user creation failed: %d %s", user.Code, user.Body.String())
	}
	var principal struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(user.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Documents","resourceId":"share-documents","resourceLabel":"Documents (share)","relativePath":"/","access":[],"protocols":[{"protocol":"smb","enabled":true},{"protocol":"nfs","enabled":false}]}`)))
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), `"resourceId"`) {
		t.Fatalf("modern create failed: %d %s", create.Code, create.Body.String())
	}
	var share struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(create.Body).Decode(&share); err != nil {
		t.Fatal(err)
	}

	access := httptest.NewRecorder()
	server.routes().ServeHTTP(access, httptest.NewRequest(http.MethodPatch, "/api/v1/shares/"+share.ID+"/access", strings.NewReader(`{"principalId":"`+principal.ID+`","level":"read"}`)))
	if access.Code != http.StatusOK || !strings.Contains(access.Body.String(), principal.ID) {
		t.Fatalf("modern access update failed: %d %s", access.Code, access.Body.String())
	}

	protocol := httptest.NewRecorder()
	server.routes().ServeHTTP(protocol, httptest.NewRequest(http.MethodPatch, "/api/v1/shares/"+share.ID+"/protocols/smb", strings.NewReader(`{"enabled":false}`)))
	if protocol.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected last protocol protection, got %d %s", protocol.Code, protocol.Body.String())
	}

	settings := httptest.NewRecorder()
	server.routes().ServeHTTP(settings, httptest.NewRequest(http.MethodPatch, "/api/v1/shares/"+share.ID, strings.NewReader(`{"description":"Updated"}`)))
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "Updated") {
		t.Fatalf("modern settings update failed: %d %s", settings.Code, settings.Body.String())
	}
}

func TestSharePartialPatchAppliesEnabledFlag(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	t.Setenv("LUMONAS_SAMBA_CONFIG", t.TempDir()+"/generated/smb.conf")

	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Media","resourceId":"share-media","relativePath":"/","access":[],"protocols":[{"protocol":"smb","enabled":true}]}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", create.Code, create.Body.String())
	}
	var share struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(create.Body).Decode(&share); err != nil {
		t.Fatal(err)
	}
	if share.Enabled {
		t.Fatal("shares should start disabled")
	}
	patch := httptest.NewRecorder()
	server.routes().ServeHTTP(patch, httptest.NewRequest(http.MethodPatch, "/api/v1/shares/"+share.ID, strings.NewReader(`{"enabled":true}`)))
	if patch.Code != http.StatusOK {
		t.Fatalf("enable patch failed: %d %s", patch.Code, patch.Body.String())
	}
	var updated struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(patch.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled {
		t.Fatal("partial enabled patch was ignored")
	}
	// Protocols must survive the partial patch.
	var detail struct {
		Protocols []struct {
			Protocol string `json:"protocol"`
		} `json:"protocols"`
	}
	detailResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(detailResponse, httptest.NewRequest(http.MethodGet, "/api/v1/shares/"+share.ID, nil))
	if err := json.NewDecoder(detailResponse.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Protocols) != 1 || detail.Protocols[0].Protocol != "smb" {
		t.Fatalf("protocols were lost in the partial patch: %#v", detail.Protocols)
	}
}
