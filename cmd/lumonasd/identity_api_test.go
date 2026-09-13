package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/trace"
)

func TestRequestAuditPersistsTypedObservabilityFields(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/docker/stacks/stack-media/actions", nil)
	request = request.WithContext(trace.WithCorrelationID(request.Context(), "corr-audit"))
	server.recordRequestAudit(request, "operator", "docker.stack.action", "stack-media", map[string]any{
		"operationId": "op-audit",
		"planHash":    "plan-audit",
		"generation":  7,
	})
	audits, err := server.store.Audit(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected one audit record, got %#v", audits)
	}
	entry := audits[0]
	if entry.CorrelationID != "corr-audit" || entry.OperationID != "op-audit" || entry.PlanHash != "plan-audit" || entry.Generation != server.currentGeneration() || entry.ResourceType != "stack" || entry.ResourceID != "stack-media" || entry.Metadata["generation"] != float64(server.currentGeneration()) {
		t.Fatalf("audit observability fields were not persisted: %#v", entry)
	}
}

func TestRequestAuditDerivesOperationIDFromQueuedJob(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/updates/check", nil)
	server.recordRequestAudit(request, "operator", "updates.check.queued", "job-queued", map[string]any{"jobId": "job-queued"})
	audits, err := server.store.Audit(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || audits[0].OperationID != "job-queued" {
		t.Fatalf("queued job operation was not promoted into audit trace: %#v", audits)
	}
}

func TestIdentityAPISeparatesFileAndManagementUsers(t *testing.T) {
	server := testServer(t)

	fileUserResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(fileUserResponse, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"media","password":"a-file-user-password"}`)))
	if fileUserResponse.Code != http.StatusCreated {
		t.Fatalf("file user creation failed: %d %s", fileUserResponse.Code, fileUserResponse.Body.String())
	}
	var fileUser identity.Principal
	if err := json.NewDecoder(fileUserResponse.Body).Decode(&fileUser); err != nil {
		t.Fatal(err)
	}
	if fileUser.ManagementRole != identity.RoleNone || fileUser.UID == nil {
		t.Fatalf("unexpected file user %#v", fileUser)
	}

	managementResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(managementResponse, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"operator","password":"a-long-development-password","managementRole":"operator"}`)))
	if managementResponse.Code != http.StatusCreated {
		t.Fatalf("management user creation failed: %d %s", managementResponse.Code, managementResponse.Body.String())
	}
	if _, _, err := server.store.CreateSession("operator", "a-long-development-password", 0); err != nil {
		t.Fatalf("management user should be able to authenticate: %v", err)
	}
	if _, _, err := server.store.CreateSession("media", "a-long-development-password", 0); err == nil {
		t.Fatal("file-only user should not be able to authenticate to management UI")
	}
}

func TestIdentityAPIGroupMembershipAndGenerationConflict(t *testing.T) {
	server := testServer(t)

	user := httptest.NewRecorder()
	server.routes().ServeHTTP(user, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"family","password":"a-file-user-password"}`)))
	if user.Code != http.StatusCreated {
		t.Fatalf("user creation failed: %d %s", user.Code, user.Body.String())
	}
	var member identity.Principal
	if err := json.NewDecoder(user.Body).Decode(&member); err != nil {
		t.Fatal(err)
	}

	group := httptest.NewRecorder()
	server.routes().ServeHTTP(group, httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(`{"name":"household"}`)))
	if group.Code != http.StatusCreated {
		t.Fatalf("group creation failed: %d %s", group.Code, group.Body.String())
	}
	var household identity.Principal
	if err := json.NewDecoder(group.Body).Decode(&household); err != nil {
		t.Fatal(err)
	}

	members := httptest.NewRecorder()
	server.routes().ServeHTTP(members, httptest.NewRequest(http.MethodPut, "/api/v1/groups/"+household.ID+"/members", strings.NewReader(`{"memberIds":["`+member.ID+`"]}`)))
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), member.ID) {
		t.Fatalf("group membership update failed: %d %s", members.Code, members.Body.String())
	}

	conflict := httptest.NewRecorder()
	server.routes().ServeHTTP(conflict, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"stale","expectedGeneration":0}`)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected stale generation conflict, got %d: %s", conflict.Code, conflict.Body.String())
	}

	deletion := httptest.NewRecorder()
	server.routes().ServeHTTP(deletion, httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+member.ID, nil))
	if deletion.Code != http.StatusConflict {
		t.Fatalf("expected membership dependency protection, got %d: %s", deletion.Code, deletion.Body.String())
	}
}
