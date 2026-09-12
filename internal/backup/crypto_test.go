package backup

import "testing"

func TestCredentialEncryptionRoundTripAndTamper(t *testing.T) {
	input := Credentials{Username: "backup", Password: "secret", AccessKey: "access", SecretKey: "key"}
	ciphertext, err := EncryptCredentials(input, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := DecryptCredentials(ciphertext, []byte("recovery-key"))
	if err != nil || output != input {
		t.Fatalf("round trip failed: %#v %v", output, err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := DecryptCredentials(ciphertext, []byte("recovery-key")); err == nil {
		t.Fatal("tampered credentials decrypted")
	}
}
