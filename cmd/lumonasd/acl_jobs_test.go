package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestACLJobAPIValidatesPathBeforeQueueing(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/acl/jobs", strings.NewReader(`{"path":"/etc","entries":[{"principal":"family","level":"read"}],"reauthenticated":true}`)))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "allow-listed") {
		t.Fatalf("unexpected ACL validation response %d: %s", response.Code, response.Body.String())
	}
}
