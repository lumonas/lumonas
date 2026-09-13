package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersistVerifiedPublishesAtomicLatestAndVersionedCopies(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1", Generation: 7}, DesiredState: []byte("{}"), Database: []byte("sqlite")}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	first, err := PersistVerified(directory, bundle, []byte("key"), now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PersistVerified(directory, bundle, []byte("key"), now)
	if err != nil {
		t.Fatal(err)
	}
	if first.LatestPath != second.LatestPath || first.VersionedPath == second.VersionedPath {
		t.Fatalf("unexpected persisted paths: first=%#v second=%#v", first, second)
	}
	for _, path := range []string{first.LatestPath, first.VersionedPath, second.VersionedPath} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(data) != string(bundle) {
			t.Fatalf("persisted bundle differs at %s", path)
		}
		info, statErr := os.Stat(path)
		if statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("unexpected permissions at %s: %v %v", path, info, statErr)
		}
	}
	if !strings.Contains(filepath.Base(first.VersionedPath), "generation-7-20260102T030405.000000000Z") {
		t.Fatalf("versioned path is not generation-addressed: %s", first.VersionedPath)
	}
}

func TestPersistVerifiedRejectsTamperedBundleBeforeCreatingDirectory(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("sqlite")}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	bundle[len(bundle)-1] ^= 1
	directory := filepath.Join(t.TempDir(), "recovery")
	if _, err := PersistVerified(directory, bundle, []byte("key"), time.Now()); err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected verification failure, got %v", err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("tampered bundle created persistence directory: %v", err)
	}
}
