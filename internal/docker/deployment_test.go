package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployStackTearsDownWhenHealthGateFails(t *testing.T) {
	root := t.TempDir()
	service := New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		if strings.Contains(command, "down --remove-orphans") {
			return nil, nil
		}
		return nil, nil
	})
	stack := updateTestStack(t, service, "fresh")

	result, err := service.DeployStack(context.Background(), stack, func(context.Context) error {
		return errors.New("container is unhealthy")
	}, fastOptions())
	if err == nil || !result.RolledBack || !strings.Contains(result.Reason, "unhealthy") {
		t.Fatalf("expected failed install cleanup, result=%#v err=%v", result, err)
	}
}

func TestRestoreComposeOnlyAcceptsAStackWithinRoot(t *testing.T) {
	service := New(t.TempDir(), nil)
	if err := service.RestoreCompose("../escape", "services:\n  app:\n    image: example/app\n"); err == nil {
		t.Fatal("path traversal restore was accepted")
	}
	stack := updateTestStack(t, service, "restore")
	compose := "services:\n  app:\n    image: example/app:restored\n"
	if err := service.RestoreCompose(stack.Name, compose); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(service.Root, stack.Name, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != compose {
		t.Fatalf("compose was not restored exactly: %q", data)
	}
}
