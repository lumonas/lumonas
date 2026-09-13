package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/lumonas/lumonas/internal/auth"
	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
)

const passkeyCeremonyTTL = 5 * time.Minute

// passkeyUser adapts a management principal plus its stored credentials to
// the webauthn.User interface. The WebAuthn user handle is derived from the
// principal ID so it is stable and unique without extra storage.
type passkeyUser struct {
	id    []byte
	name  string
	creds []webauthn.Credential
}

func passkeyHandle(principalID string) []byte {
	digest := sha256.Sum256([]byte("lumonas-passkey:" + principalID))
	return digest[:]
}

func (u passkeyUser) WebAuthnID() []byte                         { return u.id }
func (u passkeyUser) WebAuthnName() string                       { return u.name }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u passkeyUser) WebAuthnIcon() string                       { return "" }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func passkeyUserID(endpoint string) string {
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) >= 2 && parts[0] == "users" {
		return parts[1]
	}
	return ""
}

func storedToWebauthnCredentials(records []store.PasskeyCredential) []webauthn.Credential {
	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		credentials = append(credentials, webauthn.Credential{
			ID:            record.ID,
			PublicKey:     record.PublicKey,
			Authenticator: webauthn.Authenticator{SignCount: record.SignCount},
		})
	}
	return credentials
}

// webauthnInstance derives the relying party configuration from the incoming
// request so the appliance works on localhost, LAN hostnames, and TLS names
// alike. Assertions are bound to exactly the origin the ceremony started on.
func webauthnInstance(r *http.Request) (*webauthn.WebAuthn, error) {
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	} else if strings.HasPrefix(host, "[") {
		if end := strings.IndexByte(host, ']'); end > 0 {
			host = host[1:end]
		}
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return webauthn.New(&webauthn.Config{
		RPID:          host,
		RPDisplayName: "LumoNAS",
		RPOrigins:     []string{scheme + "://" + r.Host},
	})
}

// passkeyCeremony holds the in-flight registration or assertion state between
// begin and finish; it is bound to a random cookie value and expires quickly.
type passkeyCeremony struct {
	Kind        string
	PrincipalID string
	Username    string
	Session     *webauthn.SessionData
	ExpiresAt   time.Time
}

func (s *apiServer) initPasskeyCeremonies() {
	if s.passkeyCeremonies == nil {
		s.passkeyCeremonies = make(map[string]passkeyCeremony)
	}
}

func (s *apiServer) storePasskeyCeremony(kind, principalID, username string, session *webauthn.SessionData) string {
	s.passkeyMu.Lock()
	defer s.passkeyMu.Unlock()
	s.initPasskeyCeremonies()
	token, err := auth.NewToken()
	if err != nil {
		return ""
	}
	for id, ceremony := range s.passkeyCeremonies {
		if ceremony.ExpiresAt.Before(time.Now()) {
			delete(s.passkeyCeremonies, id)
		}
	}
	s.passkeyCeremonies[token] = passkeyCeremony{Kind: kind, PrincipalID: principalID, Username: username, Session: session, ExpiresAt: time.Now().Add(passkeyCeremonyTTL)}
	return token
}

func (s *apiServer) takePasskeyCeremony(token, kind string) (passkeyCeremony, bool) {
	s.passkeyMu.Lock()
	defer s.passkeyMu.Unlock()
	s.initPasskeyCeremonies()
	ceremony, ok := s.passkeyCeremonies[token]
	if !ok {
		return passkeyCeremony{}, false
	}
	if ceremony.ExpiresAt.Before(time.Now()) {
		delete(s.passkeyCeremonies, token)
		return passkeyCeremony{}, false
	}
	// A kind mismatch must not consume the ceremony: only the matching
	// finish step (or expiry) burns the single use.
	if ceremony.Kind != kind {
		return passkeyCeremony{}, false
	}
	delete(s.passkeyCeremonies, token)
	return ceremony, true
}

func setPasskeyCeremonyCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{Name: "lumonas_webauthn", Value: value, Path: "/", MaxAge: int(passkeyCeremonyTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: os.Getenv("LUMONAS_COOKIE_SECURE") == "true"})
}

func (s *apiServer) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request, id string) {
	actor, principal, ok := s.passkeyActor(w, r)
	if !ok {
		return
	}
	if id == "self" {
		id = principal.ID
	}
	if id == "" || (principal.ID != id && actor != string(identity.RoleOwner) && actor != string(identity.RoleAdmin)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owners and admins register passkeys for other users"})
		return
	}
	target, err := s.store.Principal(id)
	if err != nil || target.Kind != identity.KindUser || target.ManagementRole == identity.RoleNone {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "management user not found"})
		return
	}
	instance, err := webauthnInstance(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	records, err := s.store.PasskeyCredentials(target.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	user := passkeyUser{id: passkeyHandle(target.ID), name: target.Name, creds: storedToWebauthnCredentials(records)}
	options, session, err := instance.BeginRegistration(user, webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementPreferred, UserVerification: protocol.VerificationPreferred}), webauthn.WithConveyancePreference(protocol.PreferNoAttestation))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	token := s.storePasskeyCeremony("register", target.ID, target.Name, session)
	if token == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ceremony state could not be created"})
		return
	}
	setPasskeyCeremonyCookie(w, token)
	writeJSON(w, http.StatusOK, options)
}

func (s *apiServer) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("lumonas_webauthn")
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "passkey ceremony cookie is missing"})
		return
	}
	ceremony, ok := s.takePasskeyCeremony(cookie.Value, "register")
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "passkey registration ceremony expired; start again"})
		return
	}
	actor, actorPrincipal, actorOK := s.passkeyActor(w, r)
	if !actorOK {
		return
	}
	if actorPrincipal.ID != ceremony.PrincipalID && actor != string(identity.RoleOwner) && actor != string(identity.RoleAdmin) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owners and admins can finish another user's passkey registration"})
		return
	}
	principal, err := s.store.Principal(ceremony.PrincipalID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "management user not found"})
		return
	}
	instance, err := webauthnInstance(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	records, err := s.store.PasskeyCredentials(principal.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	user := passkeyUser{id: passkeyHandle(principal.ID), name: principal.Name, creds: storedToWebauthnCredentials(records)}
	credential, err := instance.FinishRegistration(user, *ceremony.Session, r)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "passkey registration failed: " + err.Error()})
		return
	}
	var name string
	if r.Body != nil {
		// FinishRegistration consumed the body; the display name is
		// supplied as a query parameter to keep the verification body
		// spec-shaped.
		name = strings.TrimSpace(r.URL.Query().Get("name"))
	}
	if name == "" {
		name = "Passkey " + time.Now().UTC().Format("2006-01-02 15:04")
	}
	if err := s.store.SavePasskeyCredential(store.PasskeyCredential{
		ID:                 credential.ID,
		PublicKey:          credential.PublicKey,
		Attestation:        credential.Attestation.Object,
		ClientDataJSON:     credential.Attestation.ClientDataJSON,
		AuthenticatorData:  credential.Attestation.AuthenticatorData,
		PublicKeyAlgorithm: credential.Attestation.PublicKeyAlgorithm,
		AttestationType:    credential.AttestationType,
		AttestationFormat:  credential.AttestationFormat,
		UserID:             principal.ID,
		SignCount:          credential.Authenticator.SignCount,
		Name:               name,
		CreatedAt:          time.Now().UTC(),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "auth.passkey.registered", principal.ID, map[string]any{"name": name})
	s.publishActor(actor, "auth.passkey.registered", "info", &model.ResourceRef{Type: "user", ID: principal.ID}, map[string]any{"userId": principal.ID, "name": name})
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "name": name})
}

func (s *apiServer) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	}
	username := strings.TrimSpace(input.Username)
	instance, err := webauthnInstance(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if username == "" {
		options, session, err := instance.BeginDiscoverableLogin(webAuthnLoginOptions()...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		token := s.storePasskeyCeremony("login", "", "", session)
		if token == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ceremony state could not be created"})
			return
		}
		setPasskeyCeremonyCookie(w, token)
		writeJSON(w, http.StatusOK, options)
		return
	}
	principal, err := s.store.PrincipalByName(username)
	if err != nil || principal.Kind != identity.KindUser || principal.ManagementRole == identity.RoleNone {
		// Do not reveal whether the account exists.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no passkey sign-in available for this account"})
		return
	}
	records, err := s.store.PasskeyCredentials(principal.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(records) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no passkey sign-in available for this account"})
		return
	}
	user := passkeyUser{id: passkeyHandle(principal.ID), name: principal.Name, creds: storedToWebauthnCredentials(records)}
	options, session, err := instance.BeginLogin(user, webAuthnLoginOptions()...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	token := s.storePasskeyCeremony("login", principal.ID, principal.Name, session)
	if token == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ceremony state could not be created"})
		return
	}
	setPasskeyCeremonyCookie(w, token)
	writeJSON(w, http.StatusOK, options)
}

func webAuthnLoginOptions() []webauthn.LoginOption {
	return []webauthn.LoginOption{webauthn.WithUserVerification(protocol.VerificationPreferred)}
}

func (s *apiServer) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	// Brute-force resistance mirrors the password path.
	if !s.checkRateLimit(clientIP(r)) {
		w.Header().Set("Retry-After", "300")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, try again later"})
		return
	}
	cookie, err := r.Cookie("lumonas_webauthn")
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "passkey ceremony cookie is missing"})
		return
	}
	ceremony, ok := s.takePasskeyCeremony(cookie.Value, "login")
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "passkey sign-in ceremony expired; start again"})
		return
	}
	instance, err := webauthnInstance(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var principal identity.Principal
	var credential *webauthn.Credential
	if ceremony.PrincipalID != "" {
		principal, err = s.store.Principal(ceremony.PrincipalID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no passkey sign-in available for this account"})
			return
		}
		records, err := s.store.PasskeyCredentials(principal.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		user := passkeyUser{id: passkeyHandle(principal.ID), name: principal.Name, creds: storedToWebauthnCredentials(records)}
		credential, err = instance.FinishLogin(user, *ceremony.Session, r)
	} else {
		_, credential, err = instance.FinishPasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
			record, lookupErr := s.store.PasskeyCredential(rawID)
			if lookupErr != nil || subtle.ConstantTimeCompare(passkeyHandle(record.UserID), userHandle) != 1 {
				return nil, errors.New("passkey user handle is not recognized")
			}
			owner, lookupErr := s.store.Principal(record.UserID)
			if lookupErr != nil {
				return nil, lookupErr
			}
			credentials, lookupErr := s.store.PasskeyCredentials(owner.ID)
			if lookupErr != nil {
				return nil, lookupErr
			}
			return passkeyUser{id: passkeyHandle(owner.ID), name: owner.Name, creds: storedToWebauthnCredentials(credentials)}, nil
		}, *ceremony.Session, r)
		if err == nil && credential != nil {
			record, lookupErr := s.store.PasskeyCredential(credential.ID)
			if lookupErr != nil {
				err = lookupErr
			} else {
				principal, err = s.store.Principal(record.UserID)
			}
		}
	}
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "passkey verification failed"})
		return
	}
	if credential == nil || principal.ID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "passkey verification failed"})
		return
	}
	if credential.Authenticator.CloneWarning {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "passkey signature counter indicates credential cloning; sign-in denied"})
		return
	}
	if err := s.store.UpdatePasskeySignCount(credential.ID, credential.Authenticator.SignCount); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	token, expires, err := s.store.CreateSessionForUser(principal.ID, 12*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	csrfToken := s.issueCSRFToken(token, expires)
	http.SetCookie(w, &http.Cookie{Name: "lumonas_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: os.Getenv("LUMONAS_COOKIE_SECURE") == "true"})
	s.publishActor(principal.Name, "auth.passkey.login", "info", &model.ResourceRef{Type: "user", ID: principal.ID}, map[string]any{"userId": principal.ID, "username": principal.Name})
	writeJSON(w, http.StatusOK, map[string]any{"username": principal.Name, "expiresAt": expires, "csrfToken": csrfToken})
}

func (s *apiServer) listPasskeys(w http.ResponseWriter, r *http.Request, id string) {
	actor, principal, ok := s.passkeyActor(w, r)
	if !ok {
		return
	}
	if id == "self" {
		id = principal.ID
	}
	if principal.ID != id && actor != "owner" && actor != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owners and admins view passkeys of other users"})
		return
	}
	records, err := s.store.PasskeyCredentials(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	view := make([]map[string]any, 0, len(records))
	for _, record := range records {
		view = append(view, map[string]any{"id": base64.RawURLEncoding.EncodeToString(record.ID), "name": record.Name, "createdAt": record.CreatedAt, "signCount": record.SignCount})
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *apiServer) deletePasskey(w http.ResponseWriter, r *http.Request, id, credentialID string) {
	actor, principal, ok := s.passkeyActor(w, r)
	if !ok {
		return
	}
	if id == "self" {
		id = principal.ID
	}
	if principal.ID != id && actor != "owner" && actor != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only owners and admins remove passkeys of other users"})
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(credentialID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential id is not valid"})
		return
	}
	if err := s.store.DeletePasskeyCredential(raw, id); err != nil {
		if err == store.ErrPasskeyNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "passkey not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "auth.passkey.removed", id, nil)
	s.publishActor(actor, "auth.passkey.removed", "info", &model.ResourceRef{Type: "user", ID: id}, nil)
	writeJSON(w, http.StatusNoContent, nil)
}

// passkeyActor resolves the acting principal (not just the username) so the
// passkey endpoints can enforce self-or-admin semantics. The first return
// value is the actor's management role.
func (s *apiServer) passkeyActor(w http.ResponseWriter, r *http.Request) (string, identity.Principal, bool) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return "", identity.Principal{}, false
	}
	cookie, err := r.Cookie("lumonas_session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return "", identity.Principal{}, false
	}
	userID, err := s.store.UserIDBySession(cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		return "", identity.Principal{}, false
	}
	principal, err := s.store.Principal(userID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		return "", identity.Principal{}, false
	}
	return string(principal.ManagementRole), principal, true
}
