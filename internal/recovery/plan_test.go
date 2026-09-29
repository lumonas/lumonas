package recovery

import (
	"bytes"
	"encoding/json"
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
	if err != nil || !plan.Verified || plan.DatabaseValid || !plan.DesiredStateValid || !plan.ComposeValid || !plan.EncryptedSecrets || !contains(plan.Files, "config/shares.json") {
		t.Fatalf("unexpected restore plan: %#v err=%v", plan, err)
	}
	if len(plan.Warnings) == 0 || !contains(plan.Warnings, "database payload does not contain a valid SQLite header") {
		t.Fatalf("expected invalid database warning: %#v", plan.Warnings)
	}
}

func TestPlanWarnsOnInvalidComposePayload(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("SQLite format 3\x00staged"), Compose: map[string][]byte{"media/compose.yaml": []byte("services:\n")}}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(bundle, []byte("key"))
	if err != nil || plan.ComposeValid || !contains(plan.Warnings, "invalid Docker Compose payload: docker/stacks/media/compose.yaml") {
		t.Fatalf("expected compose warning, plan=%#v err=%v", plan, err)
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

func TestStageAppdataOnlyIncludesSelectedWorkload(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "settings.json"), []byte(`{"ok":true}`), 0o640); err != nil {
		t.Fatal(err)
	}
	archive, err := ArchiveAppdata(source, DefaultAppdataArchiveLimit)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := Create(Input{
		Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("SQLite format 3\x00staged"),
		Compose: map[string][]byte{"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n"), "photos/compose.yaml": []byte("services:\n  photos:\n    image: example/photos:latest\n")},
		Appdata: []AppdataPayload{
			{Stack: "media", ContainerPath: "/config", HostPath: "/srv/lumonas/docker/appdata/media", Archive: archive},
			{Stack: "photos", ContainerPath: "/config", HostPath: "/srv/lumonas/docker/appdata/photos", Archive: archive},
		},
	}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := StageAppdata(bundle, []byte("key"), filepath.Join(root, "staged"), "media")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(result.Directory) })
	if len(result.Files) != 2 || !contains(result.Files, "docker/stacks/media/compose.yaml") {
		t.Fatalf("stage did not contain only the selected appdata and compose file: %#v", result.Files)
	}
	if _, err := os.Stat(filepath.Join(result.Directory, "docker", "stacks", "photos", "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("unselected workload was staged: %v", err)
	}
}

func TestDatabaseDumpRoundTripAndSelectedStackStaging(t *testing.T) {
	root := t.TempDir()
	compose := []byte("services:\n  db:\n    image: example/database:latest\n")
	dump := []byte("verified-database-dump")
	bundle, err := Create(Input{
		Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("SQLite format 3\x00staged"),
		Compose:       map[string][]byte{"database/compose.yaml": compose, "other/compose.yaml": []byte("services:\n  db:\n    image: example/other:latest\n")},
		DatabaseDumps: []DatabaseDumpPayload{{Stack: "database", Container: "db", Dump: dump}, {Stack: "other", Container: "db", Dump: []byte("other-dump")}},
	}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(bundle, []byte("key"))
	if err != nil || !plan.Verified || len(plan.DatabaseDumps) != 2 || plan.DatabaseDumps[0].ArchiveBytes != int64(len(dump)) {
		t.Fatalf("database dump was not verified in the restore plan: %#v err=%v", plan, err)
	}
	stage, err := StageAppdata(bundle, []byte("key"), filepath.Join(root, "staged"), "database")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stage.Directory) })
	if !contains(stage.Files, databaseDumpPath("database", "db")) || !contains(stage.Files, "docker/stacks/database/compose.yaml") {
		t.Fatalf("selected app restore is missing its database dump or compose file: %#v", stage.Files)
	}
	if contains(stage.Files, databaseDumpPath("other", "db")) || contains(stage.Files, "docker/stacks/other/compose.yaml") {
		t.Fatalf("unselected stack data was staged: %#v", stage.Files)
	}
	stagedDump, err := os.ReadFile(filepath.Join(stage.Directory, filepath.FromSlash(databaseDumpPath("database", "db"))))
	if err != nil || !bytes.Equal(stagedDump, dump) {
		t.Fatalf("staged database dump differs: %q err=%v", stagedDump, err)
	}
}

func TestDatabaseDumpManifestSizeMustMatchPayload(t *testing.T) {
	records := []DatabaseDumpRecord{{Stack: "database", Container: "db", ArchivePath: databaseDumpPath("database", "db"), ArchiveBytes: 5}}
	manifest, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseDatabaseDumpManifest(manifest, map[string][]byte{databaseDumpPath("database", "db"): []byte("dump")}); err == nil {
		t.Fatal("a database dump manifest with a false payload size was accepted")
	}
}
