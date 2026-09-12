package main

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/model"
)

const (
	totpChallengeTTL      = 5 * time.Minute
	totpChallengeMaxTries = 5
)

type totpChallenge struct {
	UserID    string
	Username  string
	ExpiresAt time.Time
	Attempts  int
}

func (s *apiServer) initTOTPState() {
	if s.totpChallenges == nil {
		s.totpChallenges = make(map[string]totpChallenge)
	}
}

func (s *apiServer) createTOTPChallenge(userID, username string) string {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()
	s.initTOTPState()
	// Bound challenge memory: drop expired entries on each creation.
	now := time.Now()
	for id, challenge := range s.totpChallenges {
		if challenge.ExpiresAt.Before(now) {
			delete(s.totpChallenges, id)
		}
	}
	id, err := auth.NewToken()
	if err != nil {
		return ""
	}
	s.totpChallenges[id] = totpChallenge{UserID: userID, Username: username, ExpiresAt: now.Add(totpChallengeTTL)}
	return id
}

// totpChallengeAttempt validates the challenge and counts a failed attempt;
// the challenge is removed once it expires or exceeds the attempt budget.
func (s *apiServer) totpChallengeAttempt(id string) (totpChallenge, bool) {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()
	s.initTOTPState()
	challenge, ok := s.totpChallenges[id]
	if !ok || challenge.ExpiresAt.Before(time.Now()) {
		delete(s.totpChallenges, id)
		return totpChallenge{}, false
	}
	challenge.Attempts++
	if challenge.Attempts > totpChallengeMaxTries {
		delete(s.totpChallenges, id)
		return totpChallenge{}, false
	}
	s.totpChallenges[id] = challenge
	return challenge, true
}

func (s *apiServer) clearTOTPChallenge(id string) {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()
	s.initTOTPState()
	delete(s.totpChallenges, id)
}

func (s *apiServer) totpCipherKey() []byte {
	key := s.recoveryKeyString()
	if key == "" {
		return nil
	}
	digest := sha256.Sum256([]byte("lumonas-totp:" + key))
	return digest[:]
}

func (s *apiServer) setupTwoFactor(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	principal, err := s.store.Principal(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if principal.Kind != identity.KindUser || principal.ManagementRole == identity.RoleNone {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "two-factor authentication applies to management users"})
		return
	}
	key := s.totpCipherKey()
	if key == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "a recovery key is required before enabling two-factor authentication"})
		return
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	codes, err := auth.NewRecoveryCodes(8)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hashes = append(hashes, auth.HashRecoveryCode(code))
	}
	ciphertext, err := backup.EncryptCredentials(backup.Credentials{SecretKey: secret}, key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.SaveTOTPPending(id, ciphertext, hashes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "identity.2fa.setup", id, map[string]any{"username": principal.Name})
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":        secret,
		"otpauthUri":    auth.TOTPURI(secret, "LumoNAS", principal.Name),
		"recoveryCodes": codes,
	})
}

func (s *apiServer) enableTwoFactor(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Code) != 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a 6-digit code is required"})
		return
	}
	pending, _, _, _, err := s.store.TOTPRecord(id)
	if err != nil || len(pending) == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no pending two-factor enrolment; run setup first"})
		return
	}
	key := s.totpCipherKey()
	if key == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "a recovery key is required before enabling two-factor authentication"})
		return
	}
	credentials, err := backup.DecryptCredentials(pending, key)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "two-factor secret could not be decrypted; verify the recovery key"})
		return
	}
	if !auth.VerifyTOTP(credentials.SecretKey, input.Code, time.Now()) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "code is not valid for this authenticator"})
		return
	}
	if err := s.store.EnableTOTP(id, pending); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "identity.2fa.enable", id, nil)
	s.publish("identity.2fa.enabled", "warning", &model.ResourceRef{Type: "user", ID: id}, map[string]any{"userId": id})
	writeJSON(w, http.StatusOK, map[string]any{"twoFactor": true})
}

func (s *apiServer) disableTwoFactor(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	principal, err := s.store.Principal(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if err := s.store.DisableTOTP(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "identity.2fa.disable", id, map[string]any{"username": principal.Name})
	s.publish("identity.2fa.disabled", "warning", &model.ResourceRef{Type: "user", ID: id}, map[string]any{"userId": id})
	writeJSON(w, http.StatusOK, map[string]any{"twoFactor": false})
}

func (s *apiServer) loginTwoFactor(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ChallengeID string `json:"challengeId"`
		Code        string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	challenge, ok := s.totpChallengeAttempt(input.ChallengeID)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "challenge expired or unknown; sign in again"})
		return
	}
	key := s.totpCipherKey()
	_, secret, _, hashes, err := s.store.TOTPRecord(challenge.UserID)
	if err != nil || len(secret) == 0 || key == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "two-factor state is unavailable; sign in again"})
		return
	}
	credentials, err := backup.DecryptCredentials(secret, key)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "two-factor secret could not be decrypted; verify the recovery key"})
		return
	}
	verified := auth.VerifyTOTP(credentials.SecretKey, input.Code, time.Now())
	if !verified && len(input.Code) > 6 {
		for _, hash := range hashes {
			if auth.HashRecoveryCode(input.Code) == hash {
				if consumed, consumeErr := s.store.ConsumeRecoveryCode(challenge.UserID, hash); consumeErr == nil && consumed {
					verified = true
					break
				}
			}
		}
	}
	if !verified {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid verification code"})
		return
	}
	s.clearTOTPChallenge(input.ChallengeID)
	token, expires, err := s.store.CreateSessionForUser(challenge.UserID, 12*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "lumonas_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: os.Getenv("LUMONAS_COOKIE_SECURE") == "true"})
	writeJSON(w, http.StatusOK, map[string]any{"username": challenge.Username, "expiresAt": expires})
}

func twoFactorUserID(endpoint string) string {
	return path.Base(path.Dir(path.Dir(endpoint)))
}
