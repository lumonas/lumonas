package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lumonas/lumonas/internal/updates"
)

type fixture struct {
	Manifest    updates.Manifest `json:"manifest"`
	Signature   string           `json:"signature"`
	PublicKey   string           `json:"publicKey"`
	PackagePath string           `json:"packagePath"`
}

func main() {
	directory := flag.String("dir", "", "directory for the update package")
	output := flag.String("output", "", "fixture JSON output path")
	version := flag.String("version", "0.0.0-api-smoke", "update version")
	flag.Parse()
	if *directory == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "-dir and -output are required")
		os.Exit(2)
	}
	result, err := createFixture(*directory, *output, *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = result
}

func createFixture(directory, output, version string) (fixture, error) {
	if version == "" {
		return fixture{}, fmt.Errorf("version is required")
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fixture{}, fmt.Errorf("create fixture directory: %w", err)
	}
	packagePath := filepath.Join(directory, "lumonas-update.pkg")
	packageData := []byte("deterministic signed LumoNAS API smoke update\n")
	if err := os.WriteFile(packagePath, packageData, 0o600); err != nil {
		return fixture{}, fmt.Errorf("write update package: %w", err)
	}
	digest := sha256.Sum256(packageData)
	manifest := updates.Manifest{
		FormatVersion: updates.ManifestFormatVersion,
		Version:       version,
		PackageSHA256: hex.EncodeToString(digest[:]),
		PackageSize:   int64(len(packageData)),
		PublishedAt:   time.Unix(1, 0).UTC(),
	}
	canonical, err := updates.CanonicalManifest(manifest)
	if err != nil {
		return fixture{}, fmt.Errorf("canonicalize update manifest: %w", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fixture{}, fmt.Errorf("generate update signing key: %w", err)
	}
	result := fixture{
		Manifest:    manifest,
		Signature:   base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)),
		PublicKey:   hex.EncodeToString(publicKey),
		PackagePath: packagePath,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return fixture{}, fmt.Errorf("encode update fixture: %w", err)
	}
	if err := os.WriteFile(output, encoded, 0o600); err != nil {
		return fixture{}, fmt.Errorf("write update fixture: %w", err)
	}
	return result, nil
}
