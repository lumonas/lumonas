package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

// browseTestSnapshot creates a real btrfs-shaped snapshot directory so the
// browse op can be exercised against the filesystem instead of a stub.
func browseTestSnapshot(t *testing.T) (source, name string) {
	t.Helper()
	pool := t.TempDir()
	source = filepath.Join(pool, "main")
	name = "2026.09.13-10.00.00"
	snapshotDir := filepath.Join(pool, "main.snapshots", name)
	if err := os.MkdirAll(filepath.Join(snapshotDir, "media", "movies"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "media", "movies", "dune.mkv"), []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "README.txt"), []byte("snapshot"), 0o640); err != nil {
		t.Fatal(err)
	}
	return source, name
}

func snapshotBrowseRequest(source, name, subpath string) request {
	return request{
		Operation:      "snapshot.browse",
		OperationID:    "op-browse",
		PlanHash:       "hash",
		Confirmed:      true,
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
		RequestedState: map[string]any{"kind": "btrfs", "source": source, "name": name, "subpath": subpath},
	}
}

func TestSnapshotBrowseListsEntries(t *testing.T) {
	source, name := browseTestSnapshot(t)
	result := executeSnapshotOperation(snapshotBrowseRequest(source, name, ""), func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if !result.OK {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected data: %#v", result.Data)
	}
	entries, ok := data["entries"].([]map[string]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("unexpected entries: %#v", data)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		names[entry["name"].(string)] = entry["directory"].(bool)
	}
	isDir, hasMedia := names["media"]
	if !hasMedia || !isDir {
		t.Fatalf("media directory missing: %#v", entries)
	}
	if _, hasReadme := names["README.txt"]; !hasReadme {
		t.Fatalf("README.txt missing: %#v", entries)
	}

	nested := executeSnapshotOperation(snapshotBrowseRequest(source, name, "media/movies"), func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if !nested.OK {
		t.Fatalf("nested listing failed: %s", nested.Error)
	}
	nestedData := nested.Data.(map[string]any)
	nestedEntries := nestedData["entries"].([]map[string]any)
	if len(nestedEntries) != 1 || nestedEntries[0]["name"] != "dune.mkv" || nestedEntries[0]["directory"] != false {
		t.Fatalf("unexpected nested entries: %#v", nestedData)
	}
}

func TestSnapshotBrowseRejectsEscapeAndUnknownPaths(t *testing.T) {
	source, name := browseTestSnapshot(t)
	run := func(string, ...string) ([]byte, error) { return nil, nil }

	for _, subpath := range []string{"../..", "/etc", "media/../../.."} {
		if result := executeSnapshotOperation(snapshotBrowseRequest(source, name, subpath), run); result.OK {
			t.Fatalf("subpath %q must be rejected", subpath)
		}
	}
	missing := executeSnapshotOperation(snapshotBrowseRequest(source, name, "media/absent"), run)
	if missing.OK || !strings.Contains(missing.Error, "does not exist") {
		t.Fatalf("unexpected missing-path result: %#v", missing)
	}
}

func TestSnapshotBrowseIsBtrfsOnlyAndValidatesName(t *testing.T) {
	source, name := browseTestSnapshot(t)
	run := func(string, ...string) ([]byte, error) { return nil, nil }

	zfsRequest := snapshotBrowseRequest(source, name, "")
	zfsRequest.RequestedState["kind"] = "zfs"
	if result := executeSnapshotOperation(zfsRequest, run); result.OK {
		t.Fatal("zfs snapshots must not be browsable as paths")
	}

	badName := snapshotBrowseRequest(source, "../escape", "")
	if result := executeSnapshotOperation(badName, run); result.OK {
		t.Fatal("invalid snapshot name must be rejected")
	}
	if err := storage.ValidateSnapshotName(name); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotBrowseIsNotAnOperationIDMutation(t *testing.T) {
	if requiresOperationID("snapshot.browse") {
		t.Fatal("browse is read-only and must not require an operation ID")
	}
}
