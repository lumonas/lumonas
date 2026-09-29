package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOnlyFileShareLinkIsPasswordProtectedScopedAndRevocable(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, err := server.store.ManagedShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share.Path, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share.Path, "public", "hello.txt"), []byte("read only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share.Path, "private.txt"), []byte("outside scope"), 0o600); err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/file-share-links", strings.NewReader(`{"shareId":"`+shareID+`","path":"public","expiresInHours":24,"password":"very-long-private-passphrase"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", create.Code, create.Body.String())
	}
	var result struct {
		URL  string `json:"url"`
		Link struct {
			ID           string `json:"id"`
			TokenHash    string `json:"tokenHash"`
			PasswordHash string `json:"passwordHash"`
		} `json:"link"`
	}
	if err := json.NewDecoder(create.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.URL == "" || result.Link.ID == "" || result.Link.TokenHash != "" || result.Link.PasswordHash != "" {
		t.Fatalf("unexpected link response: %#v", result)
	}
	token := result.URL[strings.LastIndex(result.URL, "/")+1:]
	base := "/api/v1/public/file-share-links/" + token
	unauthorized := httptest.NewRecorder()
	server.routes().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, base+"/files", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("password-less request status %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, base+"/files", nil)
	request.Header.Set("X-Share-Password", "very-long-private-passphrase")
	listing := httptest.NewRecorder()
	server.routes().ServeHTTP(listing, request)
	if listing.Code != http.StatusOK || !strings.Contains(listing.Body.String(), "hello.txt") || strings.Contains(listing.Body.String(), "private.txt") {
		t.Fatalf("scoped listing status %d: %s", listing.Code, listing.Body.String())
	}
	outside := httptest.NewRequest(http.MethodGet, base+"/files?path=../", nil)
	outside.Header.Set("X-Share-Password", "very-long-private-passphrase")
	blocked := httptest.NewRecorder()
	server.routes().ServeHTTP(blocked, outside)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("scope traversal status %d: %s", blocked.Code, blocked.Body.String())
	}
	downloadRequest := httptest.NewRequest(http.MethodGet, base+"/download?path=&name=hello.txt", nil)
	downloadRequest.Header.Set("X-Share-Password", "very-long-private-passphrase")
	download := httptest.NewRecorder()
	server.routes().ServeHTTP(download, downloadRequest)
	if download.Code != http.StatusOK || download.Body.String() != "read only" {
		t.Fatalf("download status %d: %s", download.Code, download.Body.String())
	}
	badPassword := httptest.NewRequest(http.MethodGet, base+"/files", nil)
	badPassword.Header.Set("X-Share-Password", "wrong-password")
	denied := httptest.NewRecorder()
	server.routes().ServeHTTP(denied, badPassword)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password accepted: %d", denied.Code)
	}
	revoke := httptest.NewRecorder()
	server.routes().ServeHTTP(revoke, httptest.NewRequest(http.MethodDelete, "/api/v1/file-share-links/"+result.Link.ID, nil))
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke status %d: %s", revoke.Code, revoke.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, base+"/files", nil)
	request.Header.Set("X-Share-Password", "very-long-private-passphrase")
	after := httptest.NewRecorder()
	server.routes().ServeHTTP(after, request)
	if after.Code != http.StatusNotFound {
		t.Fatalf("revoked link still active: %d", after.Code)
	}
}

func TestReadOnlyFileShareLinkRequiresDirectoryAndStrongOptionalPassword(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, _ := server.store.ManagedShare(shareID)
	_ = os.WriteFile(filepath.Join(share.Path, "file.txt"), []byte("x"), 0o600)
	for _, body := range []string{`{"shareId":"` + shareID + `","path":"file.txt","expiresInHours":1}`, `{"shareId":"` + shareID + `","path":"/","expiresInHours":1,"password":"short"}`} {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/file-share-links", strings.NewReader(body)))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid link config accepted: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestPasswordProtectedShareLinkRateLimitsFailuresPerClient(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, err := server.store.ManagedShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share.Path, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/file-share-links", strings.NewReader(`{"shareId":"`+shareID+`","path":"public","expiresInHours":24,"password":"very-long-private-passphrase"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", create.Code, create.Body.String())
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(create.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	token := result.URL[strings.LastIndex(result.URL, "/")+1:]
	endpoint := "/api/v1/public/file-share-links/" + token + "/files"
	for attempt := 0; attempt < 10; attempt++ {
		request := httptest.NewRequest(http.MethodGet, endpoint, nil)
		request.RemoteAddr = "203.0.113.45:43210"
		request.Header.Set("X-Share-Password", "wrong-password")
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d expected 401, got %d: %s", attempt+1, response.Code, response.Body.String())
		}
	}
	blocked := httptest.NewRequest(http.MethodGet, endpoint, nil)
	blocked.RemoteAddr = "203.0.113.45:43210"
	blocked.Header.Set("X-Share-Password", "very-long-private-passphrase")
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, blocked)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "900" {
		t.Fatalf("expected protected link rate limit, got %d: %s", response.Code, response.Body.String())
	}
}
