package recovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type PersistResult struct {
	Manifest      Manifest `json:"manifest"`
	LatestPath    string   `json:"latestPath"`
	VersionedPath string   `json:"versionedPath"`
}

// PersistVerified atomically publishes a verified bundle as both latest.mrb
// and a collision-safe generation copy. No new copy is published until the
// bundle passes full verification, and a failed latest rename removes the
// newly-created versioned copy when possible.
func PersistVerified(directory string, bundle, key []byte, now time.Time) (PersistResult, error) {
	if directory == "" {
		return PersistResult{}, errors.New("recovery directory is required")
	}
	manifest, err := Verify(bundle, key)
	if err != nil {
		return PersistResult{}, fmt.Errorf("recovery bundle verification failed: %w", err)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return PersistResult{}, fmt.Errorf("create recovery directory: %w", err)
	}
	directory = filepath.Clean(directory)
	latestPath := filepath.Join(directory, "latest.mrb")
	versionedPath, err := nextVersionedPath(directory, manifest.Generation, now)
	if err != nil {
		return PersistResult{}, err
	}
	latestTemp, err := writeBundleTemp(directory, ".latest-", bundle)
	if err != nil {
		return PersistResult{}, fmt.Errorf("stage latest recovery bundle: %w", err)
	}
	cleanupLatest := true
	defer func() {
		if cleanupLatest {
			_ = os.Remove(latestTemp)
		}
	}()
	versionedTemp, err := writeBundleTemp(directory, ".versioned-", bundle)
	if err != nil {
		return PersistResult{}, fmt.Errorf("stage versioned recovery bundle: %w", err)
	}
	cleanupVersioned := true
	defer func() {
		if cleanupVersioned {
			_ = os.Remove(versionedTemp)
		}
	}()
	if err := os.Rename(versionedTemp, versionedPath); err != nil {
		return PersistResult{}, fmt.Errorf("publish versioned recovery bundle: %w", err)
	}
	cleanupVersioned = false
	if err := syncDirectory(directory); err != nil {
		return PersistResult{}, fmt.Errorf("sync versioned recovery bundle: %w", err)
	}
	if err := os.Rename(latestTemp, latestPath); err != nil {
		_ = os.Remove(versionedPath)
		_ = syncDirectory(directory)
		return PersistResult{}, fmt.Errorf("publish latest recovery bundle: %w", err)
	}
	cleanupLatest = false
	if err := syncDirectory(directory); err != nil {
		return PersistResult{}, fmt.Errorf("sync latest recovery bundle: %w", err)
	}
	return PersistResult{Manifest: manifest, LatestPath: latestPath, VersionedPath: versionedPath}, nil
}

func writeBundleTemp(directory, pattern string, bundle []byte) (string, error) {
	temporary, err := os.CreateTemp(directory, pattern+"*.mrb")
	if err != nil {
		return "", err
	}
	path := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(path)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return "", err
	}
	if _, err := temporary.Write(bundle); err != nil {
		cleanup()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func nextVersionedPath(directory string, generation int64, now time.Time) (string, error) {
	base := fmt.Sprintf("generation-%d-%s", generation, now.UTC().Format("20060102T150405.000000000Z"))
	for suffix := 0; suffix < 1000; suffix++ {
		name := base
		if suffix > 0 {
			name += "-" + strconv.Itoa(suffix)
		}
		path := filepath.Join(directory, name+".mrb")
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect versioned recovery path: %w", err)
		}
	}
	return "", errors.New("could not allocate a unique versioned recovery path")
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
