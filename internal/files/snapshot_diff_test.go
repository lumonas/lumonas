package files

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompareSnapshotTreeClassifiesMetadataChanges(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "snapshot")
	current := filepath.Join(root, "current")
	for _, dir := range []string{old, current} {
		if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stamp := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	write := func(path, contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(old, "same.txt"), "same")
	write(filepath.Join(current, "same.txt"), "same")
	write(filepath.Join(old, "nested", "deleted.txt"), "old")
	write(filepath.Join(old, "modified.txt"), "old")
	write(filepath.Join(current, "modified.txt"), "new-value")
	write(filepath.Join(current, "added.txt"), "new")

	diff, err := CompareSnapshotTree(old, current)
	if err != nil {
		t.Fatal(err)
	}
	if diff.SnapshotFiles != 3 || diff.CurrentFiles != 3 || diff.Added != 1 || diff.Deleted != 1 || diff.Modified != 1 || diff.Unchanged != 1 {
		t.Fatalf("unexpected summary: %+v", diff)
	}
	if len(diff.Changes) != 3 || diff.ReviewRecommended {
		t.Fatalf("unexpected change samples or review warning: %+v", diff)
	}
}

func TestCompareSnapshotTreeDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "snapshot")
	current := filepath.Join(root, "current")
	outside := filepath.Join(root, "outside")
	for _, dir := range []string{old, current, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(current, "escape")); err != nil {
		t.Fatal(err)
	}
	diff, err := CompareSnapshotTree(old, current)
	if err != nil {
		t.Fatal(err)
	}
	if diff.CurrentFiles != 0 || diff.Deleted != 0 {
		t.Fatalf("symlink target should be ignored, got %+v", diff)
	}
}

func TestCompareSnapshotTreeRecommendsReviewForMassDeletion(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "snapshot")
	current := filepath.Join(root, "current")
	for _, dir := range []string{old, current} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 60; i++ {
		if err := os.WriteFile(filepath.Join(old, fmt.Sprintf("file-%03d", i)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	diff, err := CompareSnapshotTree(old, current)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Deleted != 60 || !diff.ReviewRecommended || len(diff.Changes) != 60 {
		t.Fatalf("mass deletion should recommend review with bounded samples: %+v", diff)
	}
}

func TestEnsureDirectoryCreatesOnlyInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := EnsureDirectory(root, "Recovered from snapshot/Photos/2026"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "Recovered from snapshot", "Photos", "2026")); err != nil || !info.IsDir() {
		t.Fatalf("recovery folders were not created: info=%v err=%v", info, err)
	}
	if err := EnsureDirectory(root, "../outside"); err == nil {
		t.Fatal("directory creation accepted path traversal")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDirectory(root, "link/escaped"); err == nil {
		t.Fatal("directory creation followed a symlink inside the share")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("directory was created outside the share: %v", err)
	}
}
