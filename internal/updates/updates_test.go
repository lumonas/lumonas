package updates

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testManifest(t *testing.T, packagePath string) Manifest {
	t.Helper()
	data, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return Manifest{FormatVersion: ManifestFormatVersion, Version: "0.2.0", PackageSHA256: fmtDigest(digest[:]), PackageSize: int64(len(data)), PublishedAt: time.Now().UTC()}
}

func fmtDigest(value []byte) string {
	const hexDigits = "0123456789abcdef"
	encoded := make([]byte, len(value)*2)
	for i, b := range value {
		encoded[i*2], encoded[i*2+1] = hexDigits[b>>4], hexDigits[b&15]
	}
	return string(encoded)
}

func TestSignedManifestAndPackageVerification(t *testing.T) {
	root := t.TempDir()
	packagePath := filepath.Join(root, "update.pkg")
	if err := os.WriteFile(packagePath, []byte("signed package"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := testManifest(t, packagePath)
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	canonical, err := CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, canonical)
	if err := VerifyManifest(publicKey, manifest, signature); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPackage(packagePath, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSignature(base64.StdEncoding.EncodeToString(signature)); err != nil {
		t.Fatal(err)
	}
	if err := VerifyManifest(publicKey, manifest, []byte("bad")); err == nil {
		t.Fatal("tampered signature was accepted")
	}
}

func TestABManagerStagesAndRollsBackAtomically(t *testing.T) {
	root := t.TempDir()
	packagePath := filepath.Join(root, "update.pkg")
	if err := os.WriteFile(packagePath, []byte("signed package"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := testManifest(t, packagePath)
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	signed, _ := CanonicalManifest(manifest)
	manager := &Manager{Root: filepath.Join(root, "state")}
	state, err := manager.StageAndActivate(packagePath, manifest, ed25519.Sign(privateKey, signed), publicKey)
	if err != nil || state.PendingSlot != "b" {
		t.Fatalf("stage failed: %#v %v", state, err)
	}
	if _, err := os.Stat(filepath.Join(root, "state", "slot-b", "package")); err != nil {
		t.Fatal(err)
	}
	healthy, err := manager.MarkHealthy(manifest.Version)
	if err != nil || healthy.ActiveSlot != "b" || healthy.PendingSlot != "" {
		t.Fatalf("health confirmation failed: %#v %v", healthy, err)
	}
	rolledBack, err := manager.Rollback("health check failed")
	if err != nil || rolledBack.ActiveSlot != "a" || rolledBack.LastError == "" {
		t.Fatalf("rollback failed: %#v %v", rolledBack, err)
	}
}
