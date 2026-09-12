package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
)

type loginResponse struct {
	CSRFToken string `json:"csrfToken"`
}

// loginAs performs a password login and returns the session cookie and CSRF
// token bound to it.
func loginAs(t *testing.T, server *apiServer, username, password string) (*http.Cookie, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	var payload loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "lumonas_session" {
			return cookie, payload.CSRFToken
		}
	}
	t.Fatal("login did not set a session cookie")
	return nil, ""
}

func TestCSRFTokenIsSessionBound(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	createManagementUser(t, server, "csrf-owner", "correct-horse-battery")

	_, tokenA := loginAs(t, server, "csrf-owner", "correct-horse-battery")
	cookieB, tokenB := loginAs(t, server, "csrf-owner", "correct-horse-battery")

	// Session A's token must not authorise session B's mutation. Logout is
	// CSRF-exempt, so use a regular mutation endpoint.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"kind":"user","name":"intruder","password":"wrong-horse-battery"}`))
	req.AddCookie(cookieB)
	req.Header.Set("X-CSRF-Token", tokenA)
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-session CSRF rejection, got %d: %s", rec.Code, rec.Body.String())
	}

	// The matching token still works (401/422 from the handler is fine —
	// the CSRF gate is what we are proving).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"kind":"user","name":"second-user","password":"correct-horse-battery"}`))
	req.AddCookie(cookieB)
	req.Header.Set("X-CSRF-Token", tokenB)
	server.routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("matching-session CSRF token was rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLogoutDeletesCSRFTokens(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	createManagementUser(t, server, "logout-owner", "correct-horse-battery")

	cookie, token := loginAs(t, server, "logout-owner", "correct-horse-battery")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", token)
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout failed: %d", rec.Code)
	}
	server.csrfMu.Lock()
	remaining := len(server.csrfTokens)
	server.csrfMu.Unlock()
	if remaining != 0 {
		t.Fatalf("logout left %d CSRF token(s) behind", remaining)
	}
}

func TestCSRFEndpointReissuesAfterRestart(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	createManagementUser(t, server, "restart-owner", "correct-horse-battery")

	cookie, _ := loginAs(t, server, "restart-owner", "correct-horse-battery")

	// Simulate a daemon restart: in-memory CSRF tokens are gone while the
	// SQLite session survives. GET /auth/csrf must reissue a usable token.
	server.csrfMu.Lock()
	server.csrfTokens = make(map[string]csrfBinding)
	server.csrfMu.Unlock()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	req.AddCookie(cookie)
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csrf reissue failed: %d %s", rec.Code, rec.Body.String())
	}
	var payload loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.CSRFToken == "" {
		t.Fatal("csrf reissue returned no token")
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", payload.CSRFToken)
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reissued csrf token rejected: %d", rec.Code)
	}
}

func TestPasswordChangeRevokesSessions(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	principal := createManagementUser(t, server, "rotate-owner", "correct-horse-battery")

	cookie, token := loginAs(t, server, "rotate-owner", "correct-horse-battery")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/"+principal.ID+"/password", strings.NewReader(`{"password":"fresh-correct-horse-battery"}`))
	req.AddCookie(cookie)
	req.Header.Set("X-CSRF-Token", token)
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("password change failed: %d %s", rec.Code, rec.Body.String())
	}

	// The session created before rotation must be dead.
	if _, ok := server.store.SessionUser(cookie.Value); ok {
		t.Fatal("session survived password rotation")
	}
}

func TestRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	server := testServer(t)
	server.authRequired = true

	// A direct (non-loopback) connection with rotating spoofed XFF values
	// must exhaust the budget for the socket address, not evade it.
	limited := false
	for i := 0; i < 8; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"x","password":"y"}`))
		req.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113."+string(rune('0'+i)))
		server.routes().ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("spoofed X-Forwarded-For values bypassed the login rate limit")
	}
}

func TestCORSDeniesUnlistedOriginsByDefault(t *testing.T) {
	server := testServer(t)
	if len(server.corsOrigins) != 0 {
		t.Skip("custom CORS origins configured")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	req.Header.Set("Origin", "https://evil.example")
	server.routes().ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unlisted origin received CORS grant: %q", got)
	}

	// An explicitly allowlisted origin still gets the grant.
	server.corsOrigins = []string{"https://admin.example"}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	req.Header.Set("Origin", "https://admin.example")
	server.routes().ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.example" {
		t.Fatalf("allowlisted origin denied CORS: %q", got)
	}
}

func TestIssueCSRFTokenBindsDigestNotRawSession(t *testing.T) {
	server := testServer(t)
	token, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	csrf := server.issueCSRFToken(token, time.Now().Add(time.Minute))
	server.csrfMu.Lock()
	binding, ok := server.csrfTokens[csrf]
	server.csrfMu.Unlock()
	if !ok {
		t.Fatal("token was not recorded")
	}
	if binding.sessionDigest != auth.TokenDigest(token) {
		t.Fatal("binding does not match the session digest")
	}
	if binding.sessionDigest == token {
		t.Fatal("raw session token stored in memory")
	}
}

func TestDynamicAuthClosesAfterFirstUser(t *testing.T) {
	server := testServer(t)
	server.dynamicAuth = true
	server.authRequired = false
	t.Setenv("LUMONAS_AUTH_REQUIRED", "")

	// Fresh install: API is open so onboarding can create the first user.
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil))
	var status struct {
		Required      bool `json:"required"`
		Configured    bool `json:"configured"`
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Required || status.Configured {
		t.Fatalf("fresh install should be open for onboarding: %#v", status)
	}

	createManagementUser(t, server, "dynamic-owner", "correct-horse-battery")

	// As soon as a management user exists the API requires a session.
	rec = httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"kind":"user","name":"nope","password":"correct-horse-battery"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after first user exists, got %d", rec.Code)
	}

	cookie, token := loginAs(t, server, "dynamic-owner", "correct-horse-battery")
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	req.AddCookie(cookie)
	server.routes().ServeHTTP(rec, req)
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.Required || !status.Authenticated {
		t.Fatalf("authenticated status wrong: %#v", status)
	}
	_ = token
}

func TestDynamicAuthExplicitEnvOptOutWins(t *testing.T) {
	server := testServer(t)
	server.dynamicAuth = true
	server.authRequired = false
	t.Setenv("LUMONAS_AUTH_REQUIRED", "false")

	createManagementUser(t, server, "optout-owner", "correct-horse-battery")

	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"kind":"user","name":"nope2","password":"correct-horse-battery"}`)))
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("explicit LUMONAS_AUTH_REQUIRED=false should keep the API open")
	}
}
