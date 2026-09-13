package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/identity"
)

func TestPasskeyUserIDExtractsOnlyUserSegment(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/users/principal-1/passkeys", want: "principal-1"},
		{path: "/users/principal-1/passkeys/register/begin", want: "principal-1"},
		{path: "/not-users/principal-1/passkeys", want: ""},
	} {
		t.Run(test.path, func(t *testing.T) {
			if got := passkeyUserID(test.path); got != test.want {
				t.Fatalf("passkeyUserID(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestPasskeyCeremonyIsSingleUseAndExpires(t *testing.T) {
	server := testServer(t)
	server.passkeyCeremonies = map[string]passkeyCeremony{
		"expired": {Kind: "login", ExpiresAt: time.Now().Add(-time.Second)},
	}
	if _, ok := server.takePasskeyCeremony("expired", "login"); ok {
		t.Fatal("expired ceremony was accepted")
	}
	token := server.storePasskeyCeremony("login", "user-1", "operator", nil)
	if token == "" {
		t.Fatal("ceremony token was empty")
	}
	if _, ok := server.takePasskeyCeremony(token, "register"); ok {
		t.Fatal("ceremony was accepted for the wrong kind")
	}
	if _, ok := server.takePasskeyCeremony(token, "login"); !ok {
		t.Fatal("valid ceremony was rejected")
	}
	if _, ok := server.takePasskeyCeremony(token, "login"); ok {
		t.Fatal("ceremony could be replayed")
	}
}

func TestPasskeyRegistrationBeginUsesStableTargetAndAllowsSelfService(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	principal, err := server.store.CreatePrincipal(identity.CreateInput{
		Kind:           identity.KindUser,
		Name:           "operator",
		Password:       "correct-horse-battery-staple",
		ManagementRole: identity.RoleOperator,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, expires, err := server.store.CreateSessionForUser(principal.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	csrf := server.issueCSRFToken(session, expires)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/users/"+principal.ID+"/passkeys/register/begin", strings.NewReader("{}"))
	request.Host = "localhost"
	request.AddCookie(&http.Cookie{Name: "lumonas_session", Value: session})
	request.Header.Set("X-CSRF-Token", csrf)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("registration begin status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"publicKey"`) {
		t.Fatalf("registration options did not contain publicKey: %s", response.Body.String())
	}
	if len(response.Result().Cookies()) == 0 || response.Result().Cookies()[0].Name != "lumonas_webauthn" {
		t.Fatalf("registration ceremony cookie was not issued: %#v", response.Result().Cookies())
	}
}

func TestPasskeyLoginBeginSupportsDiscoverableLogin(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/login/begin", strings.NewReader("{}")))
	if response.Code != http.StatusOK {
		t.Fatalf("expected public discoverable passkey endpoint, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"rpId"`) {
		t.Fatalf("discoverable options did not contain rpId: %s", response.Body.String())
	}
}
