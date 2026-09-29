package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRcloneDestinationValidationAndEncryptedConfigShape(t *testing.T) {
	base := Destination{ID: "cloud", Name: "Drive", Type: DestinationRclone, Target: "photos:lumonas", Enabled: true, Retention: DefaultRetention()}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid rclone target rejected: %v", err)
	}
	for _, target := range []string{"photos:", "../evil:path", "photos:../outside", "bad/name:path"} {
		value := base
		value.Target = target
		if err := value.Validate(); err == nil {
			t.Errorf("invalid target %q accepted", target)
		}
	}
	if err := validateRcloneConfig("[photos]\ntype = drive\ntoken = {}\n", "photos"); err != nil {
		t.Fatal(err)
	}
	for _, config := range []string{"[photos]\ntype = alias\nremote = other:path\n", "[other]\ntype = drive\n", "[photos]\ntype = ftp\n"} {
		if err := validateRcloneConfig(config, "photos"); err == nil {
			t.Errorf("unsafe/mismatched config accepted: %q", config)
		}
	}
}

func TestRcloneCopyUsesPrivateConfigAndSafeArguments(t *testing.T) {
	previous := runRcloneCommand
	t.Cleanup(func() { runRcloneCommand = previous })
	var configPath string
	var captured []string
	runRcloneCommand = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		captured = append([]string(nil), args...)
		if len(args) < 2 || args[0] != "--config" {
			t.Fatalf("unexpected rclone args: %#v", args)
		}
		configPath = args[1]
		info, err := os.Stat(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("config mode %o", info.Mode().Perm())
		}
		data, err := os.ReadFile(configPath)
		if err != nil || !strings.Contains(string(data), "token = secret") {
			t.Fatalf("credentials missing from temporary config: %v", err)
		}
		return nil, nil
	}
	source := filepath.Join(t.TempDir(), "bundle.mrb")
	if err := os.WriteFile(source, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := Destination{ID: "drive", Name: "Drive", Type: DestinationRclone, Target: "photos:LumoNAS/recovery", Retention: DefaultRetention()}
	credentials := Credentials{RcloneConfig: "[photos]\ntype = drive\ntoken = secret\n"}
	if err := Upload(context.Background(), destination, credentials, source, "recovery/generation-1.mrb"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary credential file was retained: %v", err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "copyto "+source+" photos:LumoNAS/recovery/recovery/generation-1.mrb") {
		t.Fatalf("unexpected copy target: %s", joined)
	}
}
