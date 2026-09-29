package main

import (
	"strings"

	"github.com/lumonas/lumonas/internal/storage"
)

func executeFilesystemScrub(req request, run command) response {
	if !req.Confirmed {
		return response{Error: "operation plan is not confirmed"}
	}
	kind := storage.SnapshotKind(requestedString(req.RequestedState, "filesystemKind"))
	source := requestedString(req.RequestedState, "filesystemSource")
	if err := storage.ValidateScrubSource(kind, source); err != nil {
		return response{Error: err.Error()}
	}
	pool := strings.SplitN(source, "/", 2)[0]
	switch req.Operation {
	case "filesystem.scrub.start":
		if kind == storage.SnapshotBtrfs {
			if _, err := run("btrfs", "scrub", "start", source); err != nil {
				return response{Error: "Btrfs scrub could not be started"}
			}
		} else if _, err := run("zpool", "scrub", pool); err != nil {
			return response{Error: "ZFS scrub could not be started"}
		}
		return response{OK: true, Data: map[string]string{"state": "started", "filesystemKind": string(kind), "filesystemSource": source}}
	case "filesystem.scrub.status":
		var output []byte
		var err error
		if kind == storage.SnapshotBtrfs {
			output, err = run("btrfs", "scrub", "status", "-R", source)
			if err != nil {
				return response{Error: "Btrfs scrub status could not be read"}
			}
			status, parseErr := storage.ParseBtrfsScrubStatus(string(output))
			if parseErr != nil {
				return response{Error: parseErr.Error()}
			}
			return response{OK: true, Data: status}
		}
		output, err = run("zpool", "status", pool)
		if err != nil {
			return response{Error: "ZFS scrub status could not be read"}
		}
		status, parseErr := storage.ParseZpoolScrubStatus(string(output))
		if parseErr != nil {
			return response{Error: parseErr.Error()}
		}
		return response{OK: true, Data: status}
	case "filesystem.scrub.cancel":
		if kind == storage.SnapshotBtrfs {
			if _, err := run("btrfs", "scrub", "cancel", source); err != nil {
				return response{Error: "Btrfs scrub could not be cancelled"}
			}
		} else if _, err := run("zpool", "scrub", "-s", pool); err != nil {
			return response{Error: "ZFS scrub could not be cancelled"}
		}
		return response{OK: true, Data: map[string]string{"state": "cancelled", "filesystemKind": string(kind), "filesystemSource": source}}
	default:
		return response{Error: "unsupported filesystem scrub operation"}
	}
}
