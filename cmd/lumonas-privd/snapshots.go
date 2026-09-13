package main

import (
	"context"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

// executeSnapshotOperation handles the allow-listed snapshot operations.
// RequestedState carries kind (btrfs|zfs), source, and — for list/delete —
// the snapshot name; create derives the name from an optional label and the
// current UTC timestamp so a snapshot can never overwrite another.
func executeSnapshotOperation(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	kind := storage.SnapshotKind(requestedString(req.RequestedState, "kind"))
	source := requestedString(req.RequestedState, "source")
	name := requestedString(req.RequestedState, "name")
	label := requestedString(req.RequestedState, "label")
	if err := storage.ValidateSnapshotSource(kind, source); err != nil {
		return response{Error: err.Error()}
	}
	snapshotRun := storage.SnapshotRunner(func(_ context.Context, name string, args ...string) ([]byte, error) {
		return run(name, args...)
	})
	switch req.Operation {
	case "snapshot.create":
		if err := storage.ValidateSnapshotLabel(label); err != nil {
			return response{Error: err.Error()}
		}
		snapshotName := storage.SnapshotName(label, time.Now().UTC())
		switch kind {
		case storage.SnapshotBtrfs:
			if err := storage.CreateBtrfsSnapshot(context.Background(), snapshotRun, source, snapshotName); err != nil {
				return response{Error: "btrfs snapshot creation failed"}
			}
		case storage.SnapshotZfs:
			if err := storage.CreateZfsSnapshot(context.Background(), snapshotRun, source, snapshotName); err != nil {
				return response{Error: "zfs snapshot creation failed"}
			}
		default:
			return response{Error: "unsupported snapshot kind"}
		}
		return response{OK: true, Data: storage.Snapshot{ID: string(kind) + "/" + source + "@" + snapshotName, Kind: kind, Source: source, Name: snapshotName, CreatedAt: time.Now().UTC(), Readonly: true}}
	case "snapshot.list":
		if name != "" {
			return response{Error: "snapshot list does not accept a name"}
		}
		switch kind {
		case storage.SnapshotBtrfs:
			snapshots, err := storage.ListBtrfsSnapshots(context.Background(), snapshotRun, source)
			if err != nil {
				return response{Error: "btrfs snapshot listing failed"}
			}
			return response{OK: true, Data: snapshots}
		case storage.SnapshotZfs:
			snapshots, err := storage.ListZfsSnapshots(context.Background(), snapshotRun, source)
			if err != nil {
				return response{Error: "zfs snapshot listing failed"}
			}
			return response{OK: true, Data: snapshots}
		default:
			return response{Error: "unsupported snapshot kind"}
		}
	case "snapshot.delete":
		if strings.TrimSpace(name) == "" {
			return response{Error: "snapshot name is required"}
		}
		switch kind {
		case storage.SnapshotBtrfs:
			if err := storage.DeleteBtrfsSnapshot(context.Background(), snapshotRun, source, name); err != nil {
				return response{Error: "btrfs snapshot deletion failed"}
			}
		case storage.SnapshotZfs:
			if err := storage.DeleteZfsSnapshot(context.Background(), snapshotRun, source, name); err != nil {
				return response{Error: "zfs snapshot deletion failed"}
			}
		default:
			return response{Error: "unsupported snapshot kind"}
		}
		return response{OK: true, Data: map[string]string{"deleted": name}}
	default:
		return response{Error: "operation is not allow-listed"}
	}
}
