package main

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

func snapshotRequest(operation string, state map[string]any) request {
	return request{
		Operation:      operation,
		OperationID:    "op-snapshot",
		PlanHash:       "hash",
		RequestedState: state,
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
		Confirmed:      true,
	}
}

func TestSnapshotCreateBuildsReadOnlyCommands(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	result := execute(snapshotRequest("snapshot.create", map[string]any{"kind": "btrfs", "source": "/srv/pool", "label": "nightly"}), nil, run)
	if !result.OK {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	if len(commands) != 1 || !strings.HasPrefix(commands[0], "btrfs subvolume snapshot -r /srv/pool /srv/pool.snapshots/nightly-") {
		t.Fatalf("unexpected create command: %v", commands)
	}
	snapshot, ok := result.Data.(storage.Snapshot)
	if !ok || !snapshot.Readonly || snapshot.Name == "" || snapshot.Source != "/srv/pool" {
		t.Fatalf("unexpected snapshot data: %#v", result.Data)
	}

	commands = nil
	result = execute(snapshotRequest("snapshot.create", map[string]any{"kind": "zfs", "source": "tank/media"}), nil, run)
	if !result.OK {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	if len(commands) != 1 || !strings.HasPrefix(commands[0], "zfs snapshot tank/media@") {
		t.Fatalf("unexpected zfs create command: %v", commands)
	}
}

func TestSnapshotListParsesOutput(t *testing.T) {
	run := func(name string, args ...string) ([]byte, error) {
		return []byte("ID 256 gen 10 parent 0 top level 5 otime 2026-09-12 22:00:00 path pool.snapshots/nightly-x\n"), nil
	}
	result := execute(snapshotRequest("snapshot.list", map[string]any{"kind": "btrfs", "source": "/srv/pool"}), nil, run)
	if !result.OK {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	snapshots, ok := result.Data.([]storage.Snapshot)
	if !ok || len(snapshots) != 1 || snapshots[0].Name != "nightly-x" {
		t.Fatalf("unexpected listing: %#v", result.Data)
	}

	result = execute(snapshotRequest("snapshot.list", map[string]any{"kind": "btrfs", "source": "/srv/pool", "name": "unexpected"}), nil, run)
	if result.OK {
		t.Fatal("list must reject a name field")
	}
}

func TestSnapshotDeleteRequiresConfirmedPlanAndValidName(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	result := execute(snapshotRequest("snapshot.delete", map[string]any{"kind": "zfs", "source": "tank/media", "name": "nightly-20260913T100000Z"}), nil, run)
	if !result.OK {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	if len(commands) != 1 || commands[0] != "zfs destroy tank/media@nightly-20260913T100000Z" {
		t.Fatalf("unexpected destroy command: %v", commands)
	}

	unconfirmed := snapshotRequest("snapshot.delete", map[string]any{"kind": "zfs", "source": "tank/media", "name": "x"})
	unconfirmed.Confirmed = false
	if result := execute(unconfirmed, nil, run); result.OK {
		t.Fatal("unconfirmed snapshot delete must be rejected")
	}
	escape := snapshotRequest("snapshot.delete", map[string]any{"kind": "btrfs", "source": "/srv/pool", "name": "../escape"})
	if result := execute(escape, nil, run); result.OK {
		t.Fatal("path traversal in snapshot name must be rejected")
	}
}

func TestSnapshotOperationsValidateSource(t *testing.T) {
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	for _, operation := range []string{"snapshot.create", "snapshot.list", "snapshot.delete"} {
		relative := execute(snapshotRequest(operation, map[string]any{"kind": "btrfs", "source": "relative"}), nil, run)
		if relative.OK {
			t.Fatalf("%s accepted a relative source", operation)
		}
		badKind := execute(snapshotRequest(operation, map[string]any{"kind": "ext4", "source": "/srv"}), nil, run)
		if badKind.OK {
			t.Fatalf("%s accepted an unsupported kind", operation)
		}
	}
}
