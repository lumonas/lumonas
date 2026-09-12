package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters (RFC 6238): SHA-1 HMAC, 30-second steps, 6 digits — the
// defaults every major authenticator app expects.
const (
	totpStep     = 30 * time.Second
	totpDigits   = 6
	totpDrift    = 1
	secretBytes  = 20
	recoveryCode = "lumo"
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret generates a random base32-encoded shared secret.
func NewTOTPSecret() (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return base32NoPad.EncodeToString(raw), nil
}

// NewRecoveryCodes generates n human-transcribable recovery codes.
func NewRecoveryCodes(n int) ([]string, error) {
	if n < 1 || n > 32 {
		return nil, fmt.Errorf("recovery code count %d out of range", n)
	}
	codes := make([]string, 0, n)
	for index := 0; index < n; index++ {
		groups := make([]string, 2)
		for group := range groups {
			raw := make([]byte, 3)
			if _, err := rand.Read(raw); err != nil {
				return nil, fmt.Errorf("generate recovery code: %w", err)
			}
			value := (uint32(raw[0])<<16 | uint32(raw[1])<<8 | uint32(raw[2])) % 1000000
			groups[group] = fmt.Sprintf("%06d", value)
		}
		codes = append(codes, recoveryCode+"-"+groups[0]+"-"+groups[1])
	}
	return codes, nil
}

// HashRecoveryCode returns the SHA-256 hex digest used for single-use checks.
func HashRecoveryCode(code string) string {
	digest := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return fmt.Sprintf("%x", digest)
}

func normalizeRecoveryCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

// TOTPCode computes the 6-digit code for secret at time t.
func TOTPCode(secretBase32 string, t time.Time) (string, error) {
	secret, err := base32NoPad.DecodeString(strings.ToUpper(strings.ReplaceAll(secretBase32, " ", "")))
	if err != nil {
		return "", fmt.Errorf("decode totp secret: %w", err)
	}
	return totp(secret, t.Unix()/int64(totpStep/time.Second), totpDigits), nil
}

// VerifyTOTP checks code against secret allowing one step of clock drift.
func VerifyTOTP(secretBase32, code string, now time.Time) bool {
	secret, err := base32NoPad.DecodeString(strings.ToUpper(strings.ReplaceAll(secretBase32, " ", "")))
	if err != nil || len(code) != totpDigits {
		return false
	}
	counter := now.Unix() / int64(totpStep/time.Second)
	for offset := -int64(totpDrift); offset <= int64(totpDrift); offset++ {
		expected := totp(secret, counter+offset, totpDigits)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// TOTPURI renders the otpauth:// provisioning URI for authenticator apps.
func TOTPURI(secretBase32, issuer, account string) string {
	query := url.Values{}
	query.Set("secret", secretBase32)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", "6")
	query.Set("period", "30")
	label := url.PathEscape(issuer + ":" + account)
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// totp implements the RFC 4226 HOTP algorithm over a raw secret.
func totp(secret []byte, counter int64, digits int) string {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(buffer[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (binary.BigEndian.Uint32(sum[offset : offset+4])) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, value%mod)
}
