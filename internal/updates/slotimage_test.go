package updates

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func slotImageFixture(t *testing.T) (string, Manifest, []byte, ed25519.PublicKey) {
	t.Helper()
	root := t.TempDir()
	imagePath := filepath.Join(root, "rootfs.img")
	payload := []byte("slot image payload — 1234567890abcdef")
	if err := os.WriteFile(imagePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	manifest := Manifest{FormatVersion: ManifestFormatVersion, Version: "0.3.0", PackageSHA256: fmtDigest(digest[:]), PackageSize: int64(len(payload)), PublishedAt: time.Now().UTC()}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return imagePath, manifest, ed25519.Sign(privateKey, canonical), publicKey
}

func TestStageSlotImageStagesVerifiedImageForInactiveSlot(t *testing.T) {
	imagePath, manifest, signature, publicKey := slotImageFixture(t)
	manager := &Manager{Root: t.TempDir()}

	state, err := manager.StageSlotImage(imagePath, manifest, signature, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingSlot != "b" || state.PendingVersion != "0.3.0" || state.ActiveSlot != "a" {
		t.Fatalf("unexpected state: %#v", state)
	}

	staged, stagedManifest, _, err := manager.StagedSlotImage()
	if err != nil {
		t.Fatal(err)
	}
	if staged != filepath.Join(manager.Root, "slot-b", "image") || stagedManifest.Version != "0.3.0" {
		t.Fatalf("unexpected staged image: %s %#v", staged, stagedManifest)
	}

	committed, err := manager.CommitSlotImage("0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	if committed.ActiveSlot != "b" || committed.PendingSlot != "" {
		t.Fatalf("unexpected commit state: %#v", committed)
	}
	// With B active, the next staging target must be A.
	state, err = manager.StageSlotImage(imagePath, manifest, signature, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingSlot != "a" {
		t.Fatalf("slot alternation broken: %#v", state)
	}
}

func TestStageSlotImageRejectsTamperedImage(t *testing.T) {
	imagePath, manifest, signature, publicKey := slotImageFixture(t)
	manager := &Manager{Root: t.TempDir()}
	if err := os.WriteFile(imagePath, []byte("tampered image payload — different"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StageSlotImage(imagePath, manifest, signature, publicKey); err == nil {
		t.Fatal("tampered slot image must be rejected")
	}
	if _, _, _, err := manager.StagedSlotImage(); err == nil {
		t.Fatal("staging must not have recorded anything")
	}
}

func TestCommitSlotImageRejectsWrongHealthVersion(t *testing.T) {
	imagePath, manifest, signature, publicKey := slotImageFixture(t)
	manager := &Manager{Root: t.TempDir()}
	if _, err := manager.StageSlotImage(imagePath, manifest, signature, publicKey); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CommitSlotImage("0.4.0"); err == nil {
		t.Fatal("wrong health version must not activate the pending slot")
	}
	state, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingSlot != "b" || state.ActiveSlot != "a" {
		t.Fatalf("wrong-version commit changed slot state: %#v", state)
	}
}

func TestStagedSlotImageDetectsDrift(t *testing.T) {
	imagePath, manifest, signature, publicKey := slotImageFixture(t)
	manager := &Manager{Root: t.TempDir()}
	if _, err := manager.StageSlotImage(imagePath, manifest, signature, publicKey); err != nil {
		t.Fatal(err)
	}
	staged, _, _, err := manager.StagedSlotImage()
	if err != nil {
		t.Fatal(err)
	}
	// Drift between staging and activation must fail the re-verification.
	if err := os.WriteFile(staged, []byte("drifted payload — zzzz"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := manager.StagedSlotImage(); err == nil {
		t.Fatal("drifted staged image must be rejected")
	}
}

func TestStagedSlotImageRequiresStaging(t *testing.T) {
	manager := &Manager{Root: t.TempDir()}
	if _, _, _, err := manager.StagedSlotImage(); err == nil {
		t.Fatal("activation without staging must fail")
	}
	if _, err := manager.CommitSlotImage("0.3.0"); err == nil {
		t.Fatal("commit without staging must fail")
	}
}

func TestValidateSlotBootEntry(t *testing.T) {
	for _, entry := range []string{"0", "0001", "FFFF", "00fa"} {
		if err := ValidateSlotBootEntry(entry); err != nil {
			t.Fatalf("entry %q should be valid: %v", entry, err)
		}
	}
	for _, entry := range []string{"", "nope", "12345", "-1", "zz"} {
		if err := ValidateSlotBootEntry(entry); err == nil {
			t.Fatalf("entry %q should be invalid", entry)
		}
	}
}

func TestValidateSlotDevicePathRequiresPersistentAlias(t *testing.T) {
	for _, path := range []string{"/dev/vdb", "/dev/sda", "/dev/disk/by-path/pci-0000", "/dev/disk/by-id/../vdb", "/dev/disk/by-partuuid/1234-ABCD", ""} {
		if err := ValidateSlotDevicePath(path); err == nil {
			t.Fatalf("transient or unsafe slot path accepted: %q", path)
		}
	}
	for _, path := range []string{
		"/dev/disk/by-id/virtio-LUMONAS-SLOTB",
		"/dev/disk/by-partlabel/lumonas-b",
	} {
		if err := ValidateSlotDevicePath(path); err != nil {
			t.Fatalf("persistent slot path rejected: %q: %v", path, err)
		}
	}
}
