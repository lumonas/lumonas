package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/recovery"
)

// runCLIEnv marks the re-executed child process that should behave like the
// real binary instead of like a test run.
const runCLIEnv = "LUMONAS_RECOVER_TEST_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(runCLIEnv) == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// runCLI re-executes this test binary as main(), passing os.Args, and reports
// stdout, stderr, and the exit code.
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	command := testBinaryCommand(t, args)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run CLI: %v", err)
		}
		// fatal() exits 1, and flag errors exit 2.
		code = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

func testBinaryCommand(t *testing.T, args []string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	// The child re-enters TestMain through the environment sentinel and parses
	// os.Args[1:] with the CLI's own flag package, exactly as in production.
	// #nosec G204 -- argv is constructed here, not from user input.
	command := exec.Command(executable, args...)
	command.Env = append(os.Environ(), runCLIEnv+"=1")
	return command
}

func writeFixture(t *testing.T, key []byte) (bundlePath, keyPath string) {
	t.Helper()
	dir := t.TempDir()
	bundle, err := recovery.Create(recovery.Input{
		Manifest:     recovery.Manifest{LumoNASVersion: "test", NASUUID: "nas-1", Generation: 4},
		DesiredState: []byte(`{"hostname":"recovered"}`),
		// Apply refuses to restore unless the database and Compose payloads
		// validate, so the fixture must carry a real SQLite header and a
		// parseable stack file rather than placeholder bytes.
		Database:      []byte("SQLite format 3\x00database"),
		Compose:       map[string][]byte{"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n")},
		EncryptedData: []byte("secret"),
	}, key)
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}
	bundlePath = filepath.Join(dir, "backup.mrb")
	keyPath = filepath.Join(dir, "recovery.key")
	if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return bundlePath, keyPath
}

func TestPlanRequiresBundleAndKey(t *testing.T) {
	_, stderr, code := runCLI(t)
	if code == 0 {
		t.Fatal("expected a non-zero exit when --bundle and --key-file are missing")
	}
	if !strings.Contains(stderr, "--bundle and --key-file are required") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestPlanPrintsVerifiedPlanAsJSON(t *testing.T) {
	bundlePath, keyPath := writeFixture(t, []byte("recovery-key"))
	stdout, stderr, code := runCLI(t, "--bundle", bundlePath, "--key-file", keyPath)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var plan recovery.RestorePlan
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("plan output is not valid JSON: %v (%q)", err, stdout)
	}
	if plan.Manifest.NASUUID != "nas-1" {
		t.Fatalf("plan did not describe the verified bundle: %#v", plan)
	}
}

// A wrong recovery key must fail closed, not emit a partial plan.
func TestPlanFailsClosedWithWrongKey(t *testing.T) {
	bundlePath, _ := writeFixture(t, []byte("recovery-key"))
	dir := t.TempDir()
	wrongKey := filepath.Join(dir, "wrong.key")
	if err := os.WriteFile(wrongKey, []byte("not-the-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, "--bundle", bundlePath, "--key-file", wrongKey)
	if code == 0 {
		t.Fatal("expected a non-zero exit for a mismatched recovery key")
	}
	if !strings.Contains(stderr, "verify recovery bundle") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.Contains(stderr, "nas-1") {
		t.Fatal("plan output leaked verified manifest details for an invalid bundle")
	}
}

// A relative --root must be refused by the apply path.
func TestApplyRequiresAbsoluteRoot(t *testing.T) {
	bundlePath, keyPath := writeFixture(t, []byte("recovery-key"))
	_, stderr, code := runCLI(t, "--bundle", bundlePath, "--key-file", keyPath, "--apply", "--root", "relative/root")
	if code == 0 {
		t.Fatal("expected a non-zero exit for a relative --root")
	}
	if !strings.Contains(stderr, "apply recovery bundle") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

// --apply without --root must be refused before any restore work begins.
func TestApplyRequiresRootFlag(t *testing.T) {
	bundlePath, keyPath := writeFixture(t, []byte("recovery-key"))
	_, stderr, code := runCLI(t, "--bundle", bundlePath, "--key-file", keyPath, "--apply")
	if code == 0 {
		t.Fatal("expected a non-zero exit when --apply is used without --root")
	}
	if !strings.Contains(stderr, "--root is required with --apply") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestApplyRestoresIntoAbsoluteRoot(t *testing.T) {
	bundlePath, keyPath := writeFixture(t, []byte("recovery-key"))
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--bundle", bundlePath, "--key-file", keyPath, "--apply", "--root", root)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var result recovery.ApplyResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("apply output is not valid JSON: %v (%q)", err, stdout)
	}
	if !result.Verified || !result.DatabaseRestored || !result.SecretsRestored {
		t.Fatalf("apply did not report a verified restore: %#v", result)
	}
	restored, err := os.ReadFile(filepath.Join(root, "var/lib/lumonas/lumonas.db"))
	if err != nil || !strings.HasPrefix(string(restored), "SQLite format 3") {
		t.Fatalf("database was not restored into the root: %q err=%v", restored, err)
	}
}
