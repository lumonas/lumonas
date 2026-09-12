package recovery

import (
	"testing"
)

func TestBundleRoundTripAndEncryptedPayload(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{LumoNASVersion: "test", NASUUID: "nas-1", Generation: 4}, DesiredState: []byte(`{"hostname":"nas"}`), Database: []byte("sqlite"), EncryptedData: []byte("secret")}, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := Verify(bundle, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Generation != 4 || manifest.FormatVersion != FormatVersion {
		t.Fatalf("unexpected manifest %#v", manifest)
	}
	secret, err := DecryptSecrets(bundle, []byte("recovery-key"))
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "secret" {
		t.Fatalf("unexpected secret %q", secret)
	}
}

func TestBundleTamperingFailsVerification(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{LumoNASVersion: "test"}, DesiredState: []byte("state")}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	for index := range bundle {
		if bundle[index] != 0 {
			bundle[index] ^= 1
			break
		}
	}
	if _, err := Verify(bundle, []byte("key")); err == nil {
		t.Fatal("tampered bundle verified")
	}
}
