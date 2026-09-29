package foldersync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlanCopyUpdateMirror(t *testing.T) {
	plan := BuildPlan(map[string]Entry{"new": {Size: 3}, "changed": {Size: 8}}, map[string]Entry{"changed": {Size: 2}, "extra": {Size: 4}}, true, false)
	if plan.Files != 2 || plan.Bytes != 11 || plan.Deletes != 1 || len(plan.Changes) != 3 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	copyOnly := BuildPlan(map[string]Entry{}, map[string]Entry{"extra": {Size: 4}}, false, false)
	if copyOnly.Deletes != 0 || len(copyOnly.Changes) != 0 {
		t.Fatalf("copy plan deletes destination files: %#v", copyOnly)
	}
}

func TestUSBAttachTaskMustCopyManagedShareToManagedMount(t *testing.T) {
	valid := Task{ID: "sync-usb", Name: "USB archive", Source: Endpoint{Kind: "share", ShareID: "share-photos"}, Destination: Endpoint{Kind: "mount", MountPath: "/srv/disks/disk-usb"}, Mode: "copy", OnUSBAttach: true, Enabled: true}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid USB-triggered task rejected: %v", err)
	}
	valid.Source.Kind = "destination"
	valid.Source.DestinationID = "remote-profile"
	if err := valid.Validate(); err == nil {
		t.Fatal("USB attach task accepted a remote source")
	}
	valid.Source = Endpoint{Kind: "share", ShareID: "share-photos"}
	valid.Destination = Endpoint{Kind: "share", ShareID: "share-backups"}
	if err := valid.Validate(); err == nil {
		t.Fatal("USB attach task accepted a non-mount destination")
	}
}

func TestDeepPlanFindsSameSizeAndTimeChanges(t *testing.T) {
	plan := BuildPlan(map[string]Entry{"a": {Size: 10, Hash: "one"}}, map[string]Entry{"a": {Size: 10, Hash: "two"}}, false, true)
	if len(plan.Changes) != 1 || plan.Changes[0].Action != "update" {
		t.Fatalf("deep check missed content change: %#v", plan)
	}
}

func TestApplyCopiesVerifiesAndDeletesOnlyMirrorPlan(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.MkdirAll(src, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "new"), []byte("new data"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "old"), []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	plan := Plan{Changes: []Change{{Path: "new", Action: "copy", Bytes: 8}, {Path: "old", Action: "delete"}}}
	if err := Apply(context.Background(), src, dst, plan, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "new"))
	if err != nil || string(data) != "new data" {
		t.Fatalf("copy failed: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "old")); !os.IsNotExist(err) {
		t.Fatalf("mirror deletion failed: %v", err)
	}
}

func TestApplyRejectsNestedRootsAndEscapingPaths(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := Apply(context.Background(), root, child, Plan{}, nil); err == nil {
		t.Fatal("nested roots accepted")
	}
	if _, err := safeFile(root, "../escape"); err == nil {
		t.Fatal("escaping path accepted")
	}
}

func TestScanSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	entries, err := Scan(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target was scanned: %#v", entries)
	}
}

func TestScanFilteredExcludesRelativeGlobPatterns(t *testing.T) {
	root := t.TempDir()
	for name, value := range map[string]string{
		"keep.txt":              "keep",
		"swap.tmp":              "temp",
		"cache/index.db":        "cache",
		"Photos/2025/thumbs.db": "thumbnail",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := ScanFiltered(root, false, []string{"*.tmp", "cache/**", "Photos/**/thumbs.db"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries["keep.txt"].Path != "keep.txt" {
		t.Fatalf("ignore patterns did not select only the retained file: %#v", entries)
	}
}

func TestTaskValidationRejectsUnsafeIgnorePatterns(t *testing.T) {
	task := Task{ID: "sync-ignore", Name: "Ignore", Source: Endpoint{Kind: "share", ShareID: "left"}, Destination: Endpoint{Kind: "share", ShareID: "right"}, Mode: "copy", IgnorePatterns: []string{"../outside"}}
	if err := task.Validate(); err == nil {
		t.Fatal("parent traversal ignore pattern was accepted")
	}
	task.IgnorePatterns = []string{"[invalid"}
	if err := task.Validate(); err == nil {
		t.Fatal("malformed ignore glob was accepted")
	}
}

func TestTaskValidationEnforcesLocalEndpointAndMirrorApproval(t *testing.T) {
	base := Task{ID: "sync-1", Name: "Archive", Source: Endpoint{Kind: "share", ShareID: "share-a"}, Destination: Endpoint{Kind: "destination", DestinationID: "dest-a", Prefix: "daily/archive"}, Mode: "copy", ScheduleKind: "manual"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	base.Mode = "mirror"
	if err := base.Validate(); err == nil {
		t.Fatal("mirror task without approval accepted")
	}
	base.MirrorApproved = true
	if err := base.Validate(); err != nil {
		t.Fatalf("approved mirror task rejected: %v", err)
	}
	base.Source = Endpoint{Kind: "destination", DestinationID: "dest-a"}
	if err := base.Validate(); err == nil {
		t.Fatal("remote to remote task accepted")
	}
	base.Source = Endpoint{Kind: "share", ShareID: "share-a"}
	base.Destination = Endpoint{Kind: "destination", DestinationID: "dest-a", Prefix: "../escape"}
	if err := base.Validate(); err == nil {
		t.Fatal("unsafe remote prefix accepted")
	}
}

func TestTwoWayTaskRequiresTwoLocalCopyEndpoints(t *testing.T) {
	task := Task{ID: "sync-two-way", Name: "Family folders", Direction: "two-way", Source: Endpoint{Kind: "share", ShareID: "share-a"}, Destination: Endpoint{Kind: "mount", MountPath: "/srv/disks/disk-b"}, Mode: "copy", ScheduleKind: "daily", TimeOfDay: "02:00"}
	if err := task.Validate(); err != nil {
		t.Fatalf("valid scheduled two-way task rejected: %v", err)
	}
	task.Destination = Endpoint{Kind: "destination", DestinationID: "remote"}
	if err := task.Validate(); err == nil {
		t.Fatal("two-way task accepted an SFTP/S3 endpoint")
	}
	task.Destination = Endpoint{Kind: "share", ShareID: "share-b"}
	task.Mode = "mirror"
	if err := task.Validate(); err == nil {
		t.Fatal("two-way task accepted mirror mode")
	}
	task.Mode = "copy"
	task.OnUSBAttach = true
	if err := task.Validate(); err == nil {
		t.Fatal("two-way task accepted one-sided USB attach triggering")
	}
}

func TestApplyCancellationStopsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.Mkdir(src, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Apply(ctx, src, dst, Plan{Changes: []Change{{Path: "file", Action: "copy"}}}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled transfer, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "file")); !os.IsNotExist(err) {
		t.Fatalf("canceled transfer wrote a file: %v", err)
	}
}

func TestSafeFileRejectsSymlinkedDestinationParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := safeFile(root, "link/file"); err == nil {
		t.Fatal("symlinked destination parent accepted")
	}
}

func TestBuildPlanUsesSizeAndModificationTimeBeforeDeepHash(t *testing.T) {
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	source := map[string]Entry{"file": {Size: 5, ModTime: stamp}}
	destination := map[string]Entry{"file": {Size: 5, ModTime: stamp.Add(time.Second)}}
	if plan := BuildPlan(source, destination, false, false); len(plan.Changes) != 1 {
		t.Fatalf("modification-time difference was not planned: %#v", plan)
	}
}

func TestApplyPullVerifiesDownloadsAndDefersMirrorDeletes(t *testing.T) {
	destination := t.TempDir()
	if err := os.WriteFile(filepath.Join(destination, "stale"), []byte("keep until verified"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := []byte("remote content")
	plan := Plan{Changes: []Change{{Path: "nested/file.txt", Action: "copy", Bytes: int64(len(data))}, {Path: "stale", Action: "delete"}}}
	err := ApplyPull(context.Background(), destination, plan, func(_ context.Context, name, target string) error {
		if name != "nested/file.txt" {
			t.Fatalf("unexpected remote path %q", name)
		}
		return os.WriteFile(target, data, 0o600)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(destination, "nested/file.txt"))
	if err != nil || string(actual) != string(data) {
		t.Fatalf("downloaded content %q, %v", actual, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "stale")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mirror deletion failed: %v", err)
	}
}

func TestApplyPullRejectsBadDownloadWithoutDeleting(t *testing.T) {
	destination := t.TempDir()
	stale := filepath.Join(destination, "stale")
	if err := os.WriteFile(stale, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{Changes: []Change{{Path: "file", Action: "copy", Bytes: 10, Hash: "expected"}, {Path: "stale", Action: "delete"}}}
	err := ApplyPull(context.Background(), destination, plan, func(_ context.Context, _ string, target string) error {
		return os.WriteFile(target, []byte("wrong"), 0o600)
	}, nil)
	if err == nil {
		t.Fatal("invalid download accepted")
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("deletion ran before successful transfers: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "file")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid file was promoted: %v", err)
	}
}

func TestApplyPullHonorsCancellationAndRejectsSymlinkPaths(t *testing.T) {
	destination := t.TempDir()
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(destination, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPull(context.Background(), destination, Plan{Changes: []Change{{Path: "linked/sentinel", Action: "update", Bytes: 4}}}, func(context.Context, string, string) error { return nil }, nil); err == nil {
		t.Fatal("symlink path accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ApplyPull(ctx, destination, Plan{Changes: []Change{{Path: "new", Action: "copy"}}}, func(context.Context, string, string) error { return nil }, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
