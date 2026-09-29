package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDestinationValidationAndObjectName(t *testing.T) {
	if err := (Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: "/var/lib/lumonas/recovery", Retention: DefaultRetention()}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Destination{ID: "bad", Name: "Bad", Type: DestinationLocal, Target: "../recovery", Retention: DefaultRetention()}).Validate(); err == nil {
		t.Fatal("expected unsafe local target to fail")
	}
	if err := (Destination{ID: "short", Name: "Short", Type: DestinationS3, Target: "s3://bucket/recovery", Retention: RetentionPolicy{Generations: 19, Daily: 30, Monthly: 12}}).Validate(); err == nil {
		t.Fatal("expected retention below the verified minimum to fail")
	}
	name := ObjectName(42, time.Date(2026, 9, 12, 10, 11, 12, 0, time.UTC))
	if name != "recovery/generation-42-20260912T101112Z.mrb" {
		t.Fatalf("unexpected object name %q", name)
	}
}

func TestDestinationValidationRejectsInvalidRetentionLock(t *testing.T) {
	destination := Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: "/var/lib/lumonas/recovery", Retention: DefaultRetention()}
	destination.Retention.ImmutableDays = -1
	if err := destination.Validate(); err == nil {
		t.Fatal("negative retention lock was accepted")
	}
	destination.Retention.ImmutableDays = 3651
	if err := destination.Validate(); err == nil {
		t.Fatal("overlong retention lock was accepted")
	}
}

func TestProviderObjectLockRequiresS3AndRetentionDuration(t *testing.T) {
	destination := Destination{ID: "local", Name: "Local", Type: DestinationLocal, Target: "/var/lib/lumonas/recovery", Retention: DefaultRetention()}
	destination.Retention.ProviderObjectLock = true
	if err := destination.Validate(); err == nil {
		t.Fatal("provider object lock accepted for local destination")
	}
	destination = Destination{ID: "s3", Name: "Offsite", Type: DestinationS3, Target: "https://s3.example.test/bucket", Retention: DefaultRetention()}
	destination.Retention.ProviderObjectLock = true
	if err := destination.Validate(); err == nil {
		t.Fatal("provider object lock accepted without an explicit retention period")
	}
	destination.Retention.ImmutableDays = 30
	if err := destination.Validate(); err != nil {
		t.Fatalf("valid S3 Object Lock policy rejected: %v", err)
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
