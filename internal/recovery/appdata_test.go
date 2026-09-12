package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveAndExtractAppdataRejectsSymlinksAndPreservesFiles(t *testing.T) {
	source := filepath.Join(t.TempDir(), "appdata")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "settings.db"), []byte("settings"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive, err := ArchiveAppdata(source, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "restored")
	if err := ExtractAppdata(archive, destination, 1<<20); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "nested", "settings.db"))
	if err != nil || string(data) != "settings" {
		t.Fatalf("restored appdata = %q, err=%v", data, err)
	}
}

func TestAppdataBundleRoundTripRestoresApprovedHostPath(t *testing.T) {
	source := filepath.Join(t.TempDir(), "appdata")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.yaml"), []byte("mode: safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive, err := ArchiveAppdata(source, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := Create(Input{
		Manifest:     Manifest{NASUUID: "nas-1"},
		DesiredState: []byte("{}"),
		Database:     []byte("SQLite format 3\x00database"),
		Appdata:      []AppdataPayload{{Stack: "homeassistant", ContainerPath: "/config", HostPath: "/srv/lumonas/docker/appdata/homeassistant", Archive: archive}},
	}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(bundle, []byte("key"))
	if err != nil || len(plan.Appdata) != 1 {
		t.Fatalf("unexpected appdata plan: %#v err=%v", plan, err)
	}
	root := t.TempDir()
	result, err := Apply(bundle, []byte("key"), ApplyOptions{Root: root, AppdataMaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AppdataRestored) != 1 || len(result.AppliedFiles) != 3 {
		t.Fatalf("unexpected appdata apply result: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "srv/lumonas/docker/appdata/homeassistant/config.yaml"))
	if err != nil || string(data) != "mode: safe\n" {
		t.Fatalf("restored bundle appdata = %q, err=%v", data, err)
	}
}

func TestAppdataArchiveLimitsAndUnsafePathsFailClosed(t *testing.T) {
	source := filepath.Join(t.TempDir(), "appdata")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "large.bin"), []byte(strings.Repeat("x", 4096)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveAppdata(source, 64); err == nil {
		t.Fatal("archive size limit was not enforced")
	}
	if _, err := ArchiveAppdata("/", 1<<20); err == nil {
		t.Fatal("root archive path was accepted")
	}
	if _, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("SQLite format 3\x00"), Appdata: []AppdataPayload{{Stack: "app", ContainerPath: "/config", HostPath: "/etc/passwd", Archive: []byte("bad")}}}, []byte("key")); err == nil {
		t.Fatal("unsafe appdata payload was accepted")
	}
}
