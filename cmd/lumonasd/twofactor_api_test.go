package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/store"
)

func createManagementUser(t *testing.T, server *apiServer, name, password string) identity.Principal {
	t.Helper()
	principal, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: name, Password: password, ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

func twofactorPost(server *apiServer, path, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return response
}

func TestTwoFactorEnrolmentAndLoginFlow(t *testing.T) {
	t.Setenv("LUMONAS_RECOVERY_KEY", "test-recovery-key")
	server := testServer(t)
	principal := createManagementUser(t, server, "twofactor-admin", "correct-horse-battery")

	setup := twofactorPost(server, "/api/v1/users/"+principal.ID+"/2fa/setup", `{}`)
	if setup.Code != http.StatusOK {
		t.Fatalf("setup failed %d: %s", setup.Code, setup.Body.String())
	}
	var enrolment struct {
		Secret        string   `json:"secret"`
		OTPAuthURI    string   `json:"otpauthUri"`
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.NewDecoder(setup.Body).Decode(&enrolment); err != nil {
		t.Fatal(err)
	}
	if enrolment.Secret == "" || !strings.HasPrefix(enrolment.OTPAuthURI, "otpauth://totp/") || len(enrolment.RecoveryCodes) != 8 {
		t.Fatalf("unexpected enrolment payload: %#v", enrolment)
	}

	enable := twofactorPost(server, "/api/v1/users/"+principal.ID+"/2fa/enable", `{"code":"000000"}`)
	if enable.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid code rejection, got %d", enable.Code)
	}
	code, err := auth.TOTPCode(enrolment.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	enable = twofactorPost(server, "/api/v1/users/"+principal.ID+"/2fa/enable", `{"code":"`+code+`"}`)
	if enable.Code != http.StatusOK || !strings.Contains(enable.Body.String(), `"twoFactor":true`) {
		t.Fatalf("enable failed %d: %s", enable.Code, enable.Body.String())
	}

	users := httptest.NewRecorder()
	server.routes().ServeHTTP(users, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if !strings.Contains(users.Body.String(), `"twoFactor":true`) {
		t.Fatalf("expected twoFactor true in users list: %s", users.Body.String())
	}

	// Password login now yields a challenge instead of a session.
	login := twofactorPost(server, "/api/v1/auth/login", `{"username":"twofactor-admin","password":"correct-horse-battery"}`)
	if login.Code != http.StatusAccepted || !strings.Contains(login.Body.String(), `"twoFactorRequired":true`) {
		t.Fatalf("expected challenge, got %d: %s", login.Code, login.Body.String())
	}
	var challenge struct {
		ChallengeID string `json:"challengeId"`
	}
	if err := json.NewDecoder(login.Body).Decode(&challenge); err != nil || challenge.ChallengeID == "" {
		t.Fatalf("unexpected challenge response: %#v err=%v", challenge, err)
	}

	bad := twofactorPost(server, "/api/v1/auth/login/2fa", `{"challengeId":"`+challenge.ChallengeID+`","code":"999999"}`)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid challenge code rejection, got %d: %s", bad.Code, bad.Body.String())
	}

	verify := twofactorPost(server, "/api/v1/auth/login/2fa", `{"challengeId":"`+challenge.ChallengeID+`","code":"`+code+`"}`)
	if verify.Code != http.StatusOK {
		t.Fatalf("expected verified login, got %d: %s", verify.Code, verify.Body.String())
	}
	cookies := verify.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value == "" {
		t.Fatal("expected session cookie after verification")
	}

	// A fresh password login can also be completed with a recovery code.
	login = twofactorPost(server, "/api/v1/auth/login", `{"username":"twofactor-admin","password":"correct-horse-battery"}`)
	if err := json.NewDecoder(login.Body).Decode(&challenge); err != nil {
		t.Fatal(err)
	}
	recovery := twofactorPost(server, "/api/v1/auth/login/2fa", `{"challengeId":"`+challenge.ChallengeID+`","code":"`+enrolment.RecoveryCodes[0]+`"}`)
	if recovery.Code != http.StatusOK {
		t.Fatalf("expected recovery-code login to succeed, got %d: %s", recovery.Code, recovery.Body.String())
	}
	replay := twofactorPost(server, "/api/v1/auth/login", `{"username":"twofactor-admin","password":"correct-horse-battery"}`)
	if err := json.NewDecoder(replay.Body).Decode(&challenge); err != nil {
		t.Fatal(err)
	}
	replayed := twofactorPost(server, "/api/v1/auth/login/2fa", `{"challengeId":"`+challenge.ChallengeID+`","code":"`+enrolment.RecoveryCodes[0]+`"}`)
	if replayed.Code != http.StatusUnauthorized {
		t.Fatalf("expected recovery code replay rejection, got %d", replayed.Code)
	}
}

func TestTwoFactorDisableRestoresPasswordLogin(t *testing.T) {
	t.Setenv("LUMONAS_RECOVERY_KEY", "test-recovery-key")
	server := testServer(t)
	principal := createManagementUser(t, server, "disable-admin", "correct-horse-battery")

	if response := twofactorPost(server, "/api/v1/users/"+principal.ID+"/2fa/setup", `{}`); response.Code != http.StatusOK {
		t.Fatalf("setup failed %d: %s", response.Code, response.Body.String())
	}
	if response := twofactorPost(server, "/api/v1/users/"+principal.ID+"/2fa/enable", `{"code":"000000"}`); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid code rejection, got %d", response.Code)
	}

	disable := httptest.NewRecorder()
	server.routes().ServeHTTP(disable, httptest.NewRequest(http.MethodPost, "/api/v1/users/"+principal.ID+"/2fa/disable", strings.NewReader(`{}`)))
	if disable.Code != http.StatusOK || !strings.Contains(disable.Body.String(), `"twoFactor":false`) {
		t.Fatalf("disable failed %d: %s", disable.Code, disable.Body.String())
	}
	direct := twofactorPost(server, "/api/v1/auth/login", `{"username":"disable-admin","password":"correct-horse-battery"}`)
	if direct.Code != http.StatusOK || strings.Contains(direct.Body.String(), "twoFactorRequired") {
		t.Fatalf("expected direct login after disable, got %d: %s", direct.Code, direct.Body.String())
	}
}

func TestTwoFactorSetupRequiresRecoveryKey(t *testing.T) {
	t.Setenv("LUMONAS_RECOVERY_KEY", "")
	server := testServer(t)
	principal := createManagementUser(t, server, "nokey-admin", "correct-horse-battery")
	response := twofactorPost(server, "/api/v1/users/"+principal.ID+"/2fa/setup", `{}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without recovery key, got %d: %s", response.Code, response.Body.String())
	}
	if response := twofactorPost(server, "/api/v1/users/"+principal.ID+"/2fa/enable", `{"code":"123456"}`); response.Code != http.StatusConflict {
		t.Fatalf("expected 409 enabling without enrolment, got %d", response.Code)
	}
}

func TestTwoFactorColumnsRoundTrip(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	principal, err := db.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "store-admin", Password: "correct-horse-battery", ManagementRole: identity.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if enabled, err := db.TOTPEnabled(principal.ID); err != nil || enabled {
		t.Fatalf("expected disabled by default, enabled=%v err=%v", enabled, err)
	}
	if err := db.SaveTOTPPending(principal.ID, []byte("pending-cipher"), []string{"hash-a", "hash-b"}); err != nil {
		t.Fatal(err)
	}
	pending, secret, enabled, hashes, err := db.TOTPRecord(principal.ID)
	if err != nil || string(pending) != "pending-cipher" || enabled || len(secret) != 0 || len(hashes) != 2 {
		t.Fatalf("unexpected record: pending=%q secret=%q enabled=%v hashes=%#v err=%v", pending, secret, enabled, hashes, err)
	}
	if err := db.EnableTOTP(principal.ID, []byte("active-cipher")); err != nil {
		t.Fatal(err)
	}
	if enabled, err := db.TOTPEnabled(principal.ID); err != nil || !enabled {
		t.Fatalf("expected enabled, enabled=%v err=%v", enabled, err)
	}
	consumed, err := db.ConsumeRecoveryCode(principal.ID, "hash-a")
	if err != nil || !consumed {
		t.Fatalf("expected consumption, consumed=%v err=%v", consumed, err)
	}
	if consumed, err := db.ConsumeRecoveryCode(principal.ID, "hash-a"); err != nil || consumed {
		t.Fatalf("expected replay rejection, consumed=%v err=%v", consumed, err)
	}
	if err := db.DisableTOTP(principal.ID); err != nil {
		t.Fatal(err)
	}
	if enabled, err := db.TOTPEnabled(principal.ID); err != nil || enabled {
		t.Fatalf("expected disabled after wipe, enabled=%v err=%v", enabled, err)
	}
}
