package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
	"github.com/lumonas/lumonas/internal/storage"
)

// browseEntryLimit bounds one directory listing so a snapshot with a huge
// directory cannot produce an unbounded response.
const browseEntryLimit = 500

// browseSubpath validates an optional operator path inside a snapshot: it
// must be relative, clean, and stay within the snapshot root.
func browseSubpath(subpath string) (string, error) {
	if subpath == "" || subpath == "." {
		return "", nil
	}
	if strings.HasPrefix(subpath, "/") || strings.Contains(subpath, "..") {
		return "", errors.New("snapshot path must stay inside the snapshot")
	}
	clean := filepath.Clean(subpath)
	if clean == "." || strings.HasPrefix(clean, "/") {
		return "", nil
	}
	return clean, nil
}

// browseSnapshot lists one directory inside a read-only btrfs snapshot.
// Entries are returned flat — no recursion — with a hard cap.
func browseSnapshot(source, name, subpath string) response {
	snapshotRoot := source + ".snapshots/" + name
	root := filepath.Clean(snapshotRoot)
	target := filepath.Clean(filepath.Join(root, subpath))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return response{Error: "snapshot path must stay inside the snapshot"}
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return response{Error: "snapshot path does not exist"}
		}
		return response{Error: "snapshot listing failed"}
	}
	result := make([]map[string]any, 0, len(entries))
	for index, entry := range entries {
		if index >= browseEntryLimit {
			break
		}
		item := map[string]any{"name": entry.Name(), "directory": entry.IsDir()}
		if info, infoErr := entry.Info(); infoErr == nil {
			item["sizeBytes"] = info.Size()
			item["modifiedAt"] = info.ModTime().UTC().Format(time.RFC3339)
		}
		result = append(result, item)
	}
	return response{OK: true, Data: map[string]any{
		"path":    subpath,
		"entries": result,
		"total":   len(entries),
	}}
}

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
			if _, err := run("mkdir", "-p", "--", source+".snapshots"); err != nil {
				return response{Error: "btrfs snapshot directory could not be prepared"}
			}
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
	case "snapshot.browse":
		if kind != storage.SnapshotBtrfs {
			return response{Error: "only btrfs snapshots can be browsed as paths"}
		}
		if strings.TrimSpace(name) == "" {
			return response{Error: "snapshot name is required"}
		}
		if err := storage.ValidateSnapshotName(name); err != nil {
			return response{Error: err.Error()}
		}
		subpath, err := browseSubpath(requestedString(req.RequestedState, "subpath"))
		if err != nil {
			return response{Error: err.Error()}
		}
		return browseSnapshot(source, name, subpath)
	case "snapshot.compare":
		if kind != storage.SnapshotBtrfs {
			return response{Error: "only btrfs snapshots can be compared with the current share"}
		}
		if strings.TrimSpace(name) == "" {
			return response{Error: "snapshot name is required"}
		}
		if err := storage.ValidateSnapshotName(name); err != nil {
			return response{Error: err.Error()}
		}
		diff, err := fileops.CompareSnapshotTree(source+".snapshots/"+name, source)
		if err != nil {
			return response{Error: "snapshot comparison failed: " + err.Error()}
		}
		return response{OK: true, Data: diff}
	default:
		return response{Error: "operation is not allow-listed"}
	}
}
