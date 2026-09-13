package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverStacksPreservesComposeAndFlagsRisks(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  media:\n    image: jellyfin/jellyfin:latest\n    privileged: true\n    ports:\n      - 8096:8096\n"
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte(compose), 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(root, func(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrNotExist })
	stacks, err := service.Stacks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 1 || stacks[0].Name != "media" {
		t.Fatalf("unexpected stacks %#v", stacks)
	}
	if stacks[0].ComposeYAML != compose {
		t.Fatal("compose source was not preserved")
	}
	if len(stacks[0].Risks) != 1 || stacks[0].Risks[0] != "privileged" {
		t.Fatalf("risk was not detected: %#v", stacks[0].Risks)
	}
}

func TestUnavailableServiceWithNilRunnerFailsClosed(t *testing.T) {
	if (Service{}).Available(context.Background()) {
		t.Fatal("zero-value Docker service reported availability")
	}
}

func TestContainerOutputIsNormalized(t *testing.T) {
	service := New(t.TempDir(), func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"ID":"abc","Names":"jellyfin","Image":"jellyfin:latest","State":"Up 2 hours","Ports":"0.0.0.0:8096->8096/tcp","Labels":"com.docker.compose.project=media"}` + "\n"), nil
	})
	containers, err := service.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 1 || containers[0].State != "running" || containers[0].StackID != "media" {
		t.Fatalf("unexpected container %#v", containers)
	}
}

func TestUnhealthyContainerIsNotReportedAsRunning(t *testing.T) {
	service := New(t.TempDir(), func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"ID":"abc","Names":"jellyfin","Image":"jellyfin:latest","State":"Up 2 hours (unhealthy)","Ports":"","Labels":"com.docker.compose.project=media"}` + "\n"), nil
	})
	containers, err := service.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 1 || containers[0].State != "unhealthy" {
		t.Fatalf("unexpected unhealthy container %#v", containers)
	}
}

func TestActionRejectsTraversalStackNames(t *testing.T) {
	called := false
	service := New(t.TempDir(), func(context.Context, string, ...string) ([]byte, error) {
		called = true
		return nil, nil
	})
	if err := service.Action(context.Background(), Stack{Name: "../escape"}, "start"); err == nil {
		t.Fatal("stack traversal was accepted")
	}
	if called {
		t.Fatal("Docker command ran for an invalid stack name")
	}
}
