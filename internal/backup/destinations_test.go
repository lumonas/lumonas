package backup

import (
	"context"
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
