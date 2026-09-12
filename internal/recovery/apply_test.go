package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRestoresValidatedStateAndSecrets(t *testing.T) {
	bundle, err := Create(Input{
		Manifest:     Manifest{LumoNASVersion: "test", NASUUID: "nas-1", Generation: 9},
		DesiredState: []byte(`{"hostname":"recovered"}`),
		Database:     []byte("SQLite format 3\x00database"),
		Compose:      map[string][]byte{"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n")},
		Files: map[string][]byte{
			"storage/snapraid.conf":           []byte("parity /srv/pools/parity\n"),
			"config/network-connections.json": []byte(`[{"id":"lan","interface":"eth0"}]`),
		},
		EncryptedData: []byte("recovered-secret"),
	}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	result, err := Apply(bundle, []byte("key"), ApplyOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !result.DatabaseRestored || !result.SecretsRestored || len(result.AppliedFiles) != 6 {
		t.Fatalf("unexpected apply result %#v", result)
	}
	checks := map[string]string{
		"var/lib/lumonas/lumonas.db":                           "SQLite format 3\x00database",
		"var/lib/lumonas/recovery/restored/desired-state.json": `{"hostname":"recovered"}`,
		"srv/lumonas/docker/stacks/media/compose.yaml":         "services:\n  media:\n    image: example/media:latest\n",
		"etc/lumonas/snapraid.conf":                            "parity /srv/pools/parity\n",
		"etc/lumonas/recovery/network-connections.json":        `[{"id":"lan","interface":"eth0"}]`,
		"var/lib/lumonas/secrets/recovered-secrets.bin":        "recovered-secret",
	}
	for relative, expected := range checks {
		data, readErr := os.ReadFile(filepath.Join(root, relative))
		if readErr != nil || string(data) != expected {
			t.Fatalf("restored %s = %q, err=%v", relative, data, readErr)
		}
	}
	info, err := os.Stat(filepath.Join(root, "var/lib/lumonas/secrets/recovered-secrets.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret mode = %o, want 600", info.Mode().Perm())
	}
}

func TestApplyRequiresAbsoluteRootAndRejectsSymlinkParents(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("SQLite format 3\x00")}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(bundle, []byte("key"), ApplyOptions{Root: "relative"}); err == nil {
		t.Fatal("relative recovery root was accepted")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "var/lib/lumonas"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "var")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "var")); err != nil {
		t.Fatal(err)
	}
	_, err = Apply(bundle, []byte("key"), ApplyOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink parent was not rejected: %v", err)
	}
}
