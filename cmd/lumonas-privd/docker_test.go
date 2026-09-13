package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerCommandAllowListUsesManagedPaths(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(stackDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_STACK_ROOT", root)
	var gotName string
	var gotArgs []string
	result := executeDockerCommand(request{Confirmed: true, RequestedState: map[string]any{"args": []any{"compose", "-f", composePath, "config", "--format", "json"}}}, func(name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte(`{"services":{}}`), nil
	})
	if !result.OK || gotName != "docker" || strings.Join(gotArgs, "\x00") != strings.Join([]string{"compose", "-f", composePath, "config", "--format", "json"}, "\x00") {
		t.Fatalf("unexpected Docker broker result: %#v command=%q args=%v", result, gotName, gotArgs)
	}
}

func TestDockerCommandAllowListCoversTypedRuntimeActions(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(stackDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	volumeRoot := t.TempDir()
	volumePath := filepath.Join(volumeRoot, "data")
	if err := os.WriteFile(volumePath, []byte("volume"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_STACK_ROOT", root)
	t.Setenv("LUMONAS_DOCKER_IMPORT_DIR", volumeRoot)
	for _, args := range [][]string{
		{"start", "container-1"},
		{"restart", "container-1"},
		{"volume", "inspect", "--format", "{{json .Mountpoint}}", "volume-1"},
		{"image", "inspect", "--format", "{{index .RepoDigests 0}}", "example/image:latest"},
		{"manifest", "inspect", "--verbose", "example/image:latest"},
		{"compose", "-f", composePath, "config", "--quiet"},
		{"compose", "-f", composePath, "up", "-d", "--force-recreate"},
	} {
		if err := validateDockerCommand(args); err != nil {
			t.Errorf("typed Docker command %v was rejected: %v", args, err)
		}
	}
}

func TestDockerCommandsAreUnavailableToRestrictedWorkers(t *testing.T) {
	request := request{Operation: "docker.command", PlanHash: "docker-command", Confirmed: true}
	for _, worker := range []string{"storage", "network", "power", "general"} {
		result := executeWorker(request, worker)
		if result.OK || !strings.Contains(result.Error, "not allow-listed") {
			t.Fatalf("Docker command reached restricted worker %q: %#v", worker, result)
		}
	}
}

func TestDockerCommandRejectsUntrustedOperations(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LUMONAS_STACK_ROOT", root)
	for _, args := range [][]any{
		{"exec", "container", "sh"},
		{"compose", "-f", filepath.Join(root, "../escape", "compose.yaml"), "up", "-d", "--remove-orphans"},
		{"pull", "example/image;touch /tmp/pwned"},
		{"tag", "old-id", "example/image:latest"},
	} {
		result := executeDockerCommand(request{Confirmed: true, RequestedState: map[string]any{"args": args}}, func(string, ...string) ([]byte, error) {
			t.Fatal("rejected Docker command reached the runner")
			return nil, nil
		})
		if result.OK || result.Error == "" {
			t.Fatalf("unsafe Docker command was accepted: args=%v result=%#v", args, result)
		}
	}
}

func TestDockerReadRequiresAllowListedPath(t *testing.T) {
	result := executeDockerRead(request{RequestedState: map[string]any{"path": "/containers/container-1/exec"}})
	if result.OK || !strings.Contains(result.Error, "not allow-listed") {
		t.Fatalf("unsafe Docker read was accepted: %#v", result)
	}
}
