package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

func TestCollectDockerAppdataRestartsStackAfterArchiveFailure(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  media:\n    image: example/media:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	var commands []string
	server := testServer(t)
	server.dockerService = dockerruntime.New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		if strings.Contains(command, "config --format json") {
			return []byte(`{"services":{"media":{"volumes":[{"type":"bind","source":"/srv/lumonas/missing-appdata","target":"/config"}]}}}`), nil
		}
		return nil, nil
	})
	stacks := []dockerruntime.Stack{{Name: "media", Recovery: &dockerruntime.RecoveryContract{Strategy: dockerruntime.StrategyStopBackup, AppdataPaths: []string{"/config"}}}}
	collection := server.collectDockerAppdata(context.Background(), stacks)
	if len(collection.Payloads) != 0 || len(collection.Warnings) != 1 {
		t.Fatalf("unexpected collection result: %#v", collection)
	}
	if len(commands) != 3 || !strings.HasSuffix(commands[1], " stop") || !strings.HasSuffix(commands[2], " start") {
		t.Fatalf("stack was not stopped and restarted around archive failure: %#v", commands)
	}
}

func TestCollectDockerAppdataFailsClosedForUnsupportedStrategy(t *testing.T) {
	server := testServer(t)
	server.dockerService = dockerruntime.New(t.TempDir(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		t.Fatalf("unsupported strategy executed Docker command: %s %s", name, strings.Join(args, " "))
		return nil, nil
	})
	collection := server.collectDockerAppdata(context.Background(), []dockerruntime.Stack{{
		Name:     "database",
		Recovery: &dockerruntime.RecoveryContract{Strategy: dockerruntime.StrategySnapshot, AppdataPaths: []string{"/var/lib/db"}},
	}})
	if len(collection.Payloads) != 0 || len(collection.Warnings) != 1 || !strings.Contains(collection.Warnings[0], "not executable") {
		t.Fatalf("unsupported strategy was not rejected: %#v", collection)
	}
}
