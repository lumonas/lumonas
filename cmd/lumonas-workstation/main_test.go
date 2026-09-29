package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// runCLIEnv marks the re-executed child that should behave like the released
// binary rather than like a test run.
const runCLIEnv = "LUMONAS_WORKSTATION_TEST_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(runCLIEnv) == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

func execCLI(t *testing.T, env []string, args ...string) cliResult {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	// #nosec G204 -- argv is constructed by the test, not user input.
	command := exec.Command(executable, args...)
	command.Env = append(os.Environ(), runCLIEnv+"=1")
	command.Env = append(command.Env, env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	result := cliResult{}
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run CLI: %v", err)
		}
		result.code = exitErr.ExitCode()
	}
	result.stdout, result.stderr = stdout.String(), stderr.String()
	return result
}

var base64Key = regexp.MustCompile(`^[A-Za-z0-9+/]{43}=$`)

// keygen must emit a decodable 32-byte key, since that key is what encrypts a
// workstation's backups.
func TestKeygenEmitsDecodable32ByteKey(t *testing.T) {
	result := execCLI(t, nil, "keygen")
	if result.code != 0 {
		t.Fatalf("exit %d: %s", result.code, result.stderr)
	}
	key := strings.TrimSpace(result.stdout)
	if !base64Key.MatchString(key) {
		t.Fatalf("keygen output is not a 32-byte base64 key: %q", key)
	}
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		t.Fatalf("keygen output did not decode to 32 bytes: err=%v len=%d", err, len(decoded))
	}
	// A second run must not repeat the key.
	second := execCLI(t, nil, "keygen")
	if strings.TrimSpace(second.stdout) == key {
		t.Fatal("keygen returned the same key twice")
	}
}

func TestUsageWithoutSubcommand(t *testing.T) {
	result := execCLI(t, nil)
	if result.code == 0 {
		t.Fatal("expected a non-zero exit when no subcommand is given")
	}
	if !strings.Contains(result.stdout+result.stderr, "Usage:") {
		t.Fatalf("expected usage output, got stdout=%q stderr=%q", result.stdout, result.stderr)
	}
}

// Backup/restore must refuse to run without the credentials the client needs,
// rather than silently contacting an unintended server.
func TestBackupRequiresCredentials(t *testing.T) {
	for _, missing := range []struct {
		name string
		env  map[string]string
	}{
		{"token", map[string]string{"LUMONAS_SERVER_URL": "https://nas.example.com", "LUMONAS_WORKSTATION_SHARE_ID": "share-1"}},
		{"share id", map[string]string{"LUMONAS_SERVER_URL": "https://nas.example.com", "LUMONAS_API_TOKEN": "tok"}},
	} {
		t.Run(missing.name, func(t *testing.T) {
			env := []string{
				"LUMONAS_API_TOKEN=",
				"LUMONAS_WORKSTATION_SHARE_ID=",
				"LUMONAS_SERVER_URL=",
				"LUMONAS_BACKUP_KEY=",
				"LUMONAS_BACKUP_STATE_DIR=" + t.TempDir(),
			}
			for key, value := range missing.env {
				env = append(env, key+"="+value)
			}
			result := execCLI(t, env, "backup")
			if result.code == 0 {
				t.Fatal("expected a non-zero exit when a credential is missing")
			}
			if !strings.Contains(result.stderr, "scoped API token") {
				t.Fatalf("unexpected stderr: %q", result.stderr)
			}
		})
	}
}

// A plaintext remote endpoint must be refused except for loopback development.
func TestBackupRejectsNonHTTPSServer(t *testing.T) {
	result := execCLI(t, []string{
		"LUMONAS_SERVER_URL=http://nas.example.com",
		"LUMONAS_API_TOKEN=tok",
		"LUMONAS_WORKSTATION_SHARE_ID=share-1",
		"LUMONAS_BACKUP_STATE_DIR=" + t.TempDir(),
	}, "backup")
	if result.code == 0 {
		t.Fatal("expected a non-zero exit for a plaintext server URL")
	}
	if !strings.Contains(result.stderr, "HTTPS") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

// A malformed backup key must be rejected before any network activity.
func TestBackupRejectsMalformedKey(t *testing.T) {
	result := execCLI(t, []string{
		"LUMONAS_SERVER_URL=https://nas.example.com",
		"LUMONAS_API_TOKEN=tok",
		"LUMONAS_WORKSTATION_SHARE_ID=share-1",
		"LUMONAS_BACKUP_KEY=not-a-32-byte-key",
		"LUMONAS_BACKUP_STATE_DIR=" + t.TempDir(),
	}, "backup")
	if result.code == 0 {
		t.Fatal("expected a non-zero exit for a malformed backup key")
	}
	if !strings.Contains(result.stderr, "LUMONAS_BACKUP_KEY") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}
