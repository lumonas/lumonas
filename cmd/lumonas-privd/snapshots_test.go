package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
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
	if len(commands) != 2 || commands[0] != "mkdir -p -- /srv/pool.snapshots" || !strings.HasPrefix(commands[1], "btrfs subvolume snapshot -r /srv/pool /srv/pool.snapshots/2026.") {
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
	if len(commands) != 1 || !strings.HasPrefix(commands[0], "zfs snapshot tank/media@2026.") {
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

func TestSnapshotCompareReturnsBoundedMetadataDiff(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "share")
	snapshot := source + ".snapshots/nightly"
	if err := os.MkdirAll(snapshot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := execute(snapshotRequest("snapshot.compare", map[string]any{"kind": "btrfs", "source": source, "name": "nightly"}), nil, func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if !result.OK {
		t.Fatalf("unexpected comparison failure: %s", result.Error)
	}
	diff, ok := result.Data.(fileops.SnapshotDiff)
	if !ok || diff.Added != 1 || diff.Deleted != 1 || len(diff.Changes) != 2 {
		t.Fatalf("unexpected comparison payload: %#v", result.Data)
	}
}

func TestSnapshotStreamCancellationInterruptsActiveAndPendingOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operation := registerSnapshotStreamOperation("export-cancel-test", cancel)
	// Exercise the same operation ID because stream cancellation is keyed by the
	// operation being stopped, not by the cancellation RPC itself.
	req := snapshotRequest("snapshot.export.cancel", nil)
	req.OperationID = "export-cancel-test"
	result := executeSnapshotStreamCancel(req)
	if !result.OK || ctx.Err() != context.Canceled {
		t.Fatalf("active stream was not cancelled: %#v ctx=%v", result, ctx.Err())
	}
	finishSnapshotStreamOperation("export-cancel-test", operation)

	late := snapshotRequest("snapshot.receive.cancel", nil)
	late.OperationID = "receive-cancel-late"
	result = executeSnapshotStreamCancel(late)
	if !result.OK {
		t.Fatalf("pending cancellation was rejected: %#v", result)
	}
	lateCtx, lateCancel := context.WithCancel(context.Background())
	lateOperation := registerSnapshotStreamOperation("receive-cancel-late", lateCancel)
	if lateCtx.Err() != context.Canceled {
		t.Fatal("cancel request that arrived before the Btrfs process was registered was lost")
	}
	finishSnapshotStreamOperation("receive-cancel-late", lateOperation)
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
