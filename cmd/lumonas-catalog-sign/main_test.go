package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

func TestSignCatalogWritesVerifiableDetachedSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(t.TempDir(), "apps.json")
	catalog := []byte(`[{"id":"test"}]`)
	if err := os.WriteFile(catalogPath, catalog, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := signCatalog(catalogPath, base64.StdEncoding.EncodeToString(privateKey)); err != nil {
		t.Fatal(err)
	}
	signature, err := os.ReadFile(catalogPath + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	if !dockerruntime.VerifyCatalogSignature(catalog, string(signature), base64.StdEncoding.EncodeToString(publicKey)) {
		t.Fatal("signing command emitted a signature that cannot be verified")
	}
}
