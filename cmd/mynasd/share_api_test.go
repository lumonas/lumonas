package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagedShareAPICreateUpdateDelete(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_SHARES_FILE", t.TempDir()+"/legacy-shares.json")
	t.Setenv("MYNAS_SAMBA_CONFIG", t.TempDir()+"/generated/smb.conf")

	create := httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"name":"Documents","path":"/srv/Documents","enabled":true,"protocols":["smb"],"access":{}}`))
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
}
