package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDestinationValidationAndObjectName(t *testing.T) {
	if err := (Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: "/var/lib/mynas/recovery", Retention: DefaultRetention()}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Destination{ID: "bad", Name: "Bad", Type: DestinationLocal, Target: "../recovery", Retention: DefaultRetention()}).Validate(); err == nil {
		t.Fatal("expected unsafe local target to fail")
	}
	name := ObjectName(42, time.Date(2026, 9, 12, 10, 11, 12, 0, time.UTC))
	if name != "recovery/generation-42-20260912T101112Z.mrb" {
		t.Fatalf("unexpected object name %q", name)
	}
}

func TestSHA256File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.mrb")
	if err := os.WriteFile(path, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, size, err := SHA256File(path)
	if err != nil || size != 6 || digest != "1e6ed65d77d6364eeaed5a745ba5c4985ae2b700dd85d7cf7f027bdf294a33fc" {
		t.Fatalf("unexpected digest %q size=%d err=%v", digest, size, err)
	}
}
