package notify

import (
	"bytes"
	"testing"
)

func TestNotificationCredentialsAreEncryptedAndRoundTrip(t *testing.T) {
	value := Credentials{Token: "secret-token", Username: "admin"}
	ciphertext, err := EncryptCredentials(value, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte(value.Token)) {
		t.Fatal("credential was stored in plaintext")
	}
	decoded, err := DecryptCredentials(ciphertext, []byte("recovery-key"))
	if err != nil || decoded != value {
		t.Fatalf("credential round trip failed: %#v %v", decoded, err)
	}
	if _, err := DecryptCredentials(ciphertext, []byte("wrong-key")); err == nil {
		t.Fatal("wrong key was accepted")
	}
}
