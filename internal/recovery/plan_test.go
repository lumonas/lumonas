package recovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanVerifiesBundleContents(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("sqlite"), Files: map[string][]byte{"config/shares.json": []byte("[]")}, EncryptedData: []byte("secret")}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(bundle, []byte("key"))
	if err != nil || !plan.Verified || plan.DatabaseValid || !plan.DesiredStateValid || !plan.EncryptedSecrets || !contains(plan.Files, "config/shares.json") {
		t.Fatalf("unexpected restore plan: %#v err=%v", plan, err)
	}
	if len(plan.Warnings) == 0 || !contains(plan.Warnings, "database payload does not contain a valid SQLite header") {
		t.Fatalf("expected invalid database warning: %#v", plan.Warnings)
	}
}

func TestCreateRejectsUnsafeFileName(t *testing.T) {
	if _, err := Create(Input{DesiredState: []byte("{}"), Database: []byte("db"), Files: map[string][]byte{"../secrets": []byte("x")}}, []byte("key")); err == nil {
		t.Fatal("expected unsafe file name rejection")
	}
}

func TestStageExtractsVerifiedBundleIntoPrivateDirectory(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte(`{"mode":"safe"}`), Database: []byte("SQLite format 3\x00staged"), Compose: map[string][]byte{"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n")}}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Stage(bundle, []byte("key"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Directory) })
	if !result.Verified || len(result.Files) < 3 {
		t.Fatalf("unexpected stage result %#v", result)
	}
	compose, err := os.ReadFile(filepath.Join(result.Directory, "docker", "stacks", "media", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(compose) == "" {
		t.Fatal("staged compose file is empty")
	}
}
