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
	t.Setenv("MYNAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	sambaConfig := t.TempDir() + "/generated/smb.conf"
	t.Setenv("MYNAS_SAMBA_CONFIG", sambaConfig)

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
	t.Setenv("MYNAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	directory := t.TempDir()
	t.Setenv("MYNAS_NFS_EXPORTS", directory+"/exports")
	t.Setenv("MYNAS_SFTP_CONFIG", directory+"/sftp.conf")
	t.Setenv("MYNAS_FTP_CONFIG", directory+"/ftp.conf")
	t.Setenv("MYNAS_RSYNC_CONFIG", directory+"/rsync.conf")

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
