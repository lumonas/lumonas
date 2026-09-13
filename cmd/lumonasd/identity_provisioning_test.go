package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/privileged"
)

type brokerRecord struct {
	requests []privileged.Request
	failWith error
}

func (record *brokerRecord) exec(_ context.Context, request privileged.Request) error {
	if record.failWith != nil {
		return record.failWith
	}
	record.requests = append(record.requests, request)
	return nil
}

func createFileUser(t *testing.T, server *apiServer, name, password string) *httptest.ResponseRecorder {
	t.Helper()
	payload := `{"kind":"user","name":"` + name + `","password":"` + password + `"}`
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(payload)))
	return response
}

func TestFileUserCreationProvisionsOSAndSambaAccounts(t *testing.T) {
	server := testServer(t)
	record := &brokerRecord{}
	server.brokerExec = record.exec
	response := createFileUser(t, server, "media", "a-file-user-password")
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if len(record.requests) != 2 {
		t.Fatalf("expected system and samba provisioning requests, got %#v", record.requests)
	}
	if record.requests[0].Operation != "identity.system-user.ensure" {
		t.Fatalf("expected system user provisioning first, got %q", record.requests[0].Operation)
	}
	if record.requests[0].OperationID == "" || record.requests[1].OperationID == "" {
		t.Fatalf("identity provisioning requests must carry operation IDs: %#v", record.requests)
	}
	uid := int(record.requests[0].RequestedState["uid"].(int))
	gid := int(record.requests[0].RequestedState["gid"].(int))
	if uid < 100 || uid > 60000 || gid < 100 || gid > 60000 {
		t.Fatalf("provisioned uid/gid out of range: uid=%d gid=%d", uid, gid)
	}
	if record.requests[0].RequestedState["create"] != true {
		t.Fatalf("expected create flag: %#v", record.requests[0].RequestedState)
	}
	samba := record.requests[1]
	if samba.Operation != "samba.user.ensure" || samba.RequestedState["create"] != true || samba.RequestedState["password"] != "a-file-user-password" {
		t.Fatalf("unexpected samba provisioning request: %#v", samba)
	}
	principals, err := server.store.ListPrincipals(identity.KindUser)
	if err != nil || len(principals) != 1 {
		t.Fatalf("principal should persist: %#v err %v", principals, err)
	}
	if principals[0].UID == nil || *principals[0].UID != int64(uid) {
		t.Fatalf("store uid must match provisioned uid: %#v", principals[0].UID)
	}
}

func TestFileUserCreationRollsBackWhenBrokerUnavailable(t *testing.T) {
	server := testServer(t)
	server.brokerExec = func(context.Context, privileged.Request) error {
		return context.DeadlineExceeded
	}
	response := createFileUser(t, server, "media", "a-file-user-password")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "provisioning failed") {
		t.Fatalf("expected broker failure with rollback, got %d: %s", response.Code, response.Body.String())
	}
	principals, err := server.store.ListPrincipals(identity.KindUser)
	if err != nil || len(principals) != 0 {
		t.Fatalf("principal should be rolled back: %#v err %v", principals, err)
	}
}

func TestFileUserCreationRequiresPassword(t *testing.T) {
	server := testServer(t)
	record := &brokerRecord{}
	server.brokerExec = record.exec
	response := createFileUser(t, server, "media", "")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "at least 8") {
		t.Fatalf("expected password requirement, got %d: %s", response.Code, response.Body.String())
	}
	if len(record.requests) != 0 {
		t.Fatal("no provisioning should be attempted for invalid input")
	}
}

func TestFileUserDisableSynchronizesAccounts(t *testing.T) {
	server := testServer(t)
	record := &brokerRecord{}
	server.brokerExec = record.exec
	created := createFileUser(t, server, "media", "a-file-user-password")
	if created.Code != http.StatusCreated {
		t.Fatalf("setup failed: %d %s", created.Code, created.Body.String())
	}
	var principal struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}
	disable := httptest.NewRecorder()
	server.routes().ServeHTTP(disable, httptest.NewRequest(http.MethodPatch, "/api/v1/users/"+principal.ID, strings.NewReader(`{"enabled":false}`)))
	if disable.Code != http.StatusOK {
		t.Fatalf("disable failed: %d %s", disable.Code, disable.Body.String())
	}
	if len(record.requests) != 4 {
		t.Fatalf("expected provisioning plus two disable requests, got %d", len(record.requests))
	}
	last := record.requests[3]
	if last.Operation != "samba.user.ensure" || last.RequestedState["disabled"] != true {
		t.Fatalf("expected samba disable, got %#v", last)
	}
}

func TestFileUserPasswordRotationSynchronizesSamba(t *testing.T) {
	server := testServer(t)
	record := &brokerRecord{}
	server.brokerExec = record.exec
	created := createFileUser(t, server, "media", "a-file-user-password")
	if created.Code != http.StatusCreated {
		t.Fatalf("setup failed: %d %s", created.Code, created.Body.String())
	}
	var principal struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}
	rotate := httptest.NewRecorder()
	server.routes().ServeHTTP(rotate, httptest.NewRequest(http.MethodPost, "/api/v1/users/"+principal.ID+"/password", strings.NewReader(`{"password":"rotated-file-password"}`)))
	if rotate.Code != http.StatusNoContent {
		t.Fatalf("rotation failed: %d %s", rotate.Code, rotate.Body.String())
	}
	last := record.requests[len(record.requests)-1]
	if last.Operation != "samba.user.ensure" || last.RequestedState["password"] != "rotated-file-password" {
		t.Fatalf("expected samba rotation, got %#v", last)
	}
}

func TestManagementUsersSkipOSProvisioning(t *testing.T) {
	server := testServer(t)
	record := &brokerRecord{}
	server.brokerExec = record.exec
	payload := `{"kind":"user","name":"operator","password":"a-management-password","managementRole":"operator"}`
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(payload)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if len(record.requests) != 0 {
		t.Fatalf("management users must not provision OS accounts: %#v", record.requests)
	}
}

func TestDeletedFileIdentityAccountsAreLockedBestEffort(t *testing.T) {
	server := testServer(t)
	created := createFileUser(t, server, "media", "a-file-user-password")
	if created.Code != http.StatusCreated {
		t.Fatalf("setup failed: %d %s", created.Code, created.Body.String())
	}
	var principal struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&principal); err != nil {
		t.Fatal(err)
	}
	// Broker starts failing now: the delete must still succeed.
	server.brokerExec = func(context.Context, privileged.Request) error {
		return context.DeadlineExceeded
	}
	deletion := httptest.NewRecorder()
	server.routes().ServeHTTP(deletion, httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+principal.ID, nil))
	if deletion.Code != http.StatusNoContent {
		t.Fatalf("delete must succeed even when account locking fails: %d %s", deletion.Code, deletion.Body.String())
	}
	principals, err := server.store.ListPrincipals("")
	if err != nil || len(principals) != 0 {
		t.Fatalf("principal should be gone: %#v err %v", principals, err)
	}
}
