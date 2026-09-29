// lumonas-catalog-sign creates the detached Ed25519 signature consumed by
// lumonasd. Keep LUMONAS_CATALOG_PRIVATE_KEY in the release secret store.
//
//	lumonas-catalog-sign <catalog.json>
//	lumonas-catalog-sign --verify <catalog.json> <catalog.json.sig> <base64-public-key>
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--verify" {
		if len(os.Args) != 5 {
			fmt.Fprintln(os.Stderr, "usage: lumonas-catalog-sign --verify <catalog.json> <catalog.json.sig> <base64-public-key>")
			os.Exit(2)
		}
		if err := verifyCatalog(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("catalog signature verified")
		return
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: lumonas-catalog-sign <catalog.json>")
		os.Exit(2)
	}
	if err := signCatalog(os.Args[1], os.Getenv("LUMONAS_CATALOG_PRIVATE_KEY")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// verifyCatalog independently re-checks a detached signature. It is the release
// gate that proves a tampered catalog cannot ship.
func verifyCatalog(catalogPath, signaturePath, encodedKey string) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil {
		return errors.New("catalog public key must be standard-base64 Ed25519 bytes")
	}
	if len(key) != ed25519.PublicKeySize {
		return errors.New("catalog public key must be a standard-base64 Ed25519 public key")
	}
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}
	raw, err := os.ReadFile(signaturePath)
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return errors.New("detached signature must be standard-base64")
	}
	if !ed25519.Verify(ed25519.PublicKey(key), catalog, signature) {
		return errors.New("catalog signature does not match the configured trusted key")
	}
	return nil
}

func signCatalog(path, encodedKey string) error {
	keyBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil {
		return errors.New("LUMONAS_CATALOG_PRIVATE_KEY must be standard-base64 Ed25519 private key bytes")
	}
	var privateKey ed25519.PrivateKey
	switch len(keyBytes) {
	case ed25519.SeedSize:
		privateKey = ed25519.NewKeyFromSeed(keyBytes)
	case ed25519.PrivateKeySize:
		privateKey = ed25519.PrivateKey(keyBytes)
	default:
		return errors.New("catalog signing key must contain an Ed25519 seed or private key")
	}
	catalog, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}
	signature := ed25519.Sign(privateKey, catalog)
	output := filepath.Clean(path) + ".sig"
	temporary, err := os.CreateTemp(filepath.Dir(output), ".catalog-signature-*")
	if err != nil {
		return fmt.Errorf("create temporary signature: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set detached signature permissions: %w", err)
	}
	if _, err := temporary.Write([]byte(base64.StdEncoding.EncodeToString(signature) + "\n")); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write detached signature: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync detached signature: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close detached signature: %w", err)
	}
	if err := os.Rename(temporaryPath, output); err != nil {
		return fmt.Errorf("replace detached signature: %w", err)
	}
	return nil
}
