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

func TestCollectDockerAppdataRunsHooksAndCapturesDatabaseDump(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "database")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  db:\n    image: example/database:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	var executed []string
	server := testServer(t)
	server.dockerService = dockerruntime.New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		executed = append(executed, command)
		if strings.Contains(command, "pg_dump") {
			return []byte("database-dump-content"), nil
		}
		return nil, nil
	})
	collection := server.collectDockerAppdata(context.Background(), []dockerruntime.Stack{{
		Name: "database",
		Recovery: &dockerruntime.RecoveryContract{
			Strategy:       dockerruntime.StrategyStopBackup,
			PreBackupHook:  &dockerruntime.RecoveryHook{Container: "db", Command: "flush-cache", Timeout: 10},
			PostBackupHook: &dockerruntime.RecoveryHook{Container: "db", Command: "resume-writes", Timeout: 10},
			DBDump:         &dockerruntime.RecoveryHook{Container: "db", Command: "pg_dump --format=custom", Timeout: 10},
		},
	}})
	if len(collection.DatabaseDumps) != 1 || string(collection.DatabaseDumps[0].Dump) != "database-dump-content" {
		t.Fatalf("database dump was not captured: %#v", collection)
	}
	if len(collection.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", collection.Warnings)
	}
	if len(executed) != 3 || !strings.Contains(executed[0], "flush-cache") || !strings.Contains(executed[1], "pg_dump") || !strings.Contains(executed[2], "resume-writes") {
		t.Fatalf("hooks ran in the wrong order: %#v", executed)
	}
}

func TestCollectDockerAppdataWarnsWhenDatabaseDumpFails(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "database")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  db:\n    image: example/database:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := testServer(t)
	server.dockerService = dockerruntime.New(root, func(context.Context, string, ...string) ([]byte, error) {
		return nil, os.ErrPermission
	})
	collection := server.collectDockerAppdata(context.Background(), []dockerruntime.Stack{{
		Name: "database",
		Recovery: &dockerruntime.RecoveryContract{
			Strategy: dockerruntime.StrategyStopBackup,
			DBDump:   &dockerruntime.RecoveryHook{Container: "db", Command: "pg_dump", Timeout: 10},
		},
	}})
	if len(collection.DatabaseDumps) != 0 || len(collection.Warnings) != 1 || !strings.Contains(collection.Warnings[0], "database dump failed") {
		t.Fatalf("failed database dump was not reported: %#v", collection)
	}
}
