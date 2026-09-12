package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalUploadIsAtomicAndRejectsEscapes(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.mrb")
	if err := os.WriteFile(source, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	destination := Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: target, Retention: DefaultRetention()}
	if err := Upload(context.Background(), destination, Credentials{}, source, "recovery/latest.mrb"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "recovery", "latest.mrb"))
	if err != nil || string(data) != "bundle" {
		t.Fatalf("unexpected uploaded data %q: %v", data, err)
	}
	if err := Upload(context.Background(), destination, Credentials{}, source, "../escape.mrb"); err == nil {
		t.Fatal("expected escaping object to fail")
	}
}

func TestLocalUploadAndDownloadVerificationChecksThePromotedCopy(t *testing.T) {
	source := filepath.Join(t.TempDir(), "bundle.mrb")
	data := []byte("verified bundle")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	destination := Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: t.TempDir(), Retention: DefaultRetention()}
	object := "recovery/verified.mrb"
	if err := UploadAndVerifyWithRetry(destination, Credentials{}, source, object, hex.EncodeToString(digest[:]), int64(len(data)), 2); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "downloaded.mrb")
	if err := Download(context.Background(), destination, Credentials{}, object, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(data) {
		t.Fatalf("unexpected downloaded copy %q: %v", got, err)
	}
	if err := Download(context.Background(), destination, Credentials{}, "../escape.mrb", target); err == nil {
		t.Fatal("expected unsafe remote object to be rejected")
	}
}
