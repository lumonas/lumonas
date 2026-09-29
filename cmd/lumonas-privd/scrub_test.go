package main

import (
	"reflect"
	"testing"
)

func scrubRequest(operation, kind, source string) request {
	return request{Operation: operation, Confirmed: true, RequestedState: map[string]any{"filesystemKind": kind, "filesystemSource": source}}
}

func TestExecuteFilesystemScrubUsesAllowListedCommands(t *testing.T) {
	var gotName string
	var gotArgs []string
	run := func(name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, append([]string(nil), args...)
		switch name {
		case "btrfs":
			return []byte("Status: running\n"), nil
		case "zpool":
			return []byte("pool: tank\nscan: scrub in progress since yesterday\n25.00% done\n"), nil
		default:
			return nil, nil
		}
	}
	if result := executeFilesystemScrub(scrubRequest("filesystem.scrub.start", "btrfs", "/srv/pools/media"), run); !result.OK {
		t.Fatalf("btrfs scrub start failed: %#v", result)
	}
	if gotName != "btrfs" || !reflect.DeepEqual(gotArgs, []string{"scrub", "start", "/srv/pools/media"}) {
		t.Fatalf("unexpected Btrfs start command: %s %q", gotName, gotArgs)
	}
	status := executeFilesystemScrub(scrubRequest("filesystem.scrub.status", "zfs", "tank/media"), run)
	if !status.OK || gotName != "zpool" || !reflect.DeepEqual(gotArgs, []string{"status", "tank"}) {
		t.Fatalf("unexpected ZFS status call: result=%#v command=%s %q", status, gotName, gotArgs)
	}
	if cancelled := executeFilesystemScrub(scrubRequest("filesystem.scrub.cancel", "zfs", "tank/media"), run); !cancelled.OK || !reflect.DeepEqual(gotArgs, []string{"scrub", "-s", "tank"}) {
		t.Fatalf("unexpected ZFS cancellation: result=%#v args=%q", cancelled, gotArgs)
	}
}

func TestExecuteFilesystemScrubRejectsUnconfirmedAndUnmanagedTargets(t *testing.T) {
	req := scrubRequest("filesystem.scrub.start", "btrfs", "/etc")
	called := false
	run := func(string, ...string) ([]byte, error) { called = true; return nil, nil }
	if result := executeFilesystemScrub(req, run); result.OK || called {
		t.Fatalf("unmanaged scrub target reached command runner: %#v", result)
	}
	req = scrubRequest("filesystem.scrub.start", "btrfs", "/srv/pools/media")
	req.Confirmed = false
	if result := executeFilesystemScrub(req, run); result.OK || called {
		t.Fatalf("unconfirmed scrub ran: %#v", result)
	}
}
