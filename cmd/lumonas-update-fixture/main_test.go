package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/updates"
)

func TestCreateFixtureProducesVerifiablePackage(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "fixture.json")
	result, err := createFixture(filepath.Join(root, "package"), output, "0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := hex.DecodeString(result.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := parseFixtureSignature(result.Signature)
	if err != nil {
		t.Fatal(err)
	}
	if err := updates.VerifyManifest(ed25519.PublicKey(publicKey), result.Manifest, signature); err != nil {
		t.Fatalf("fixture manifest did not verify: %v", err)
	}
	if err := updates.VerifyPackage(result.PackagePath, result.Manifest); err != nil {
		t.Fatalf("fixture package did not verify: %v", err)
	}
	if info, err := os.Stat(output); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("fixture output permissions = %v, err=%v", info, err)
	}
	var decoded fixture
	encoded, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Manifest.Version != result.Manifest.Version {
		t.Fatalf("fixture JSON = %#v, err=%v", decoded, err)
	}
}

func parseFixtureSignature(value string) ([]byte, error) {
	return updates.ParseSignature(value)
}
