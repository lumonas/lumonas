package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTOTPCodeMatchesRFC6238SixDigitVector(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	code, err := TOTPCode(secret, time.Unix(59, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if code != "287082" {
		t.Fatalf("unexpected TOTP code %q", code)
	}
	if !VerifyTOTP(secret, code, time.Unix(59, 0).UTC()) {
		t.Fatal("generated TOTP code was rejected")
	}
	if !VerifyTOTP(secret, code, time.Unix(89, 0).UTC()) {
		t.Fatal("one-step clock drift was not accepted")
	}
	if VerifyTOTP(secret, code, time.Unix(180, 0).UTC()) {
		t.Fatal("stale TOTP code was accepted")
	}
}

func TestRecoveryCodesAreUniqueAndTranscribable(t *testing.T) {
	codes, err := NewRecoveryCodes(16)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if len(code) != len("lumo-000000-000000") || !strings.HasPrefix(code, "lumo-") {
			t.Fatalf("unexpected recovery code format %q", code)
		}
		if _, exists := seen[code]; exists {
			t.Fatalf("duplicate recovery code %q", code)
		}
		seen[code] = struct{}{}
	}
}

func TestTOTPURIContainsProvisioningFields(t *testing.T) {
	uri := TOTPURI("ABC123", "LumoNAS", "admin@example.test")
	for _, expected := range []string{"otpauth://totp/", "secret=ABC123", "issuer=LumoNAS", "digits=6", "period=30"} {
		if !strings.Contains(uri, expected) {
			t.Fatalf("URI %q is missing %q", uri, expected)
		}
	}
}
