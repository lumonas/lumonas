package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxSnapshotDiffFiles = 250000
const maxSnapshotDiffSamples = 100

type SnapshotChange struct {
	Path       string    `json:"path"`
	Kind       string    `json:"kind"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type SnapshotDiff struct {
	SnapshotFiles     int              `json:"snapshotFiles"`
	CurrentFiles      int              `json:"currentFiles"`
	Added             int              `json:"added"`
	Deleted           int              `json:"deleted"`
	Modified          int              `json:"modified"`
	Unchanged         int              `json:"unchanged"`
	ReviewRecommended bool             `json:"reviewRecommended"`
	Changes           []SnapshotChange `json:"changes"`
}

type diffFile struct {
	size    int64
	modTime time.Time
}

// CompareSnapshotTree compares file size and modification time between a
// read-only snapshot and the current managed share. Symlinks and special files
// are ignored, and the walk has a hard file-count ceiling.
func CompareSnapshotTree(snapshotRoot, currentRoot string) (SnapshotDiff, error) {
	snapshot, err := scanDiffTree(snapshotRoot)
	if err != nil {
		return SnapshotDiff{}, err
	}
	current, err := scanDiffTree(currentRoot)
	if err != nil {
		return SnapshotDiff{}, err
	}
	result := SnapshotDiff{
		SnapshotFiles: len(snapshot),
		CurrentFiles:  len(current),
		Changes:       make([]SnapshotChange, 0),
	}
	paths := make(map[string]struct{}, len(snapshot)+len(current))
	for name := range snapshot {
		paths[name] = struct{}{}
	}
	for name := range current {
		paths[name] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for name := range paths {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		oldFile, existedBefore := snapshot[name]
		newFile, existsNow := current[name]
		switch {
		case existedBefore && !existsNow:
			result.Deleted++
			result.addSample(SnapshotChange{Path: name, Kind: "deleted", SizeBytes: oldFile.size, ModifiedAt: oldFile.modTime})
		case !existedBefore && existsNow:
			result.Added++
			result.addSample(SnapshotChange{Path: name, Kind: "added", SizeBytes: newFile.size, ModifiedAt: newFile.modTime})
		case oldFile.size != newFile.size || !oldFile.modTime.Equal(newFile.modTime):
			result.Modified++
			result.addSample(SnapshotChange{Path: name, Kind: "modified", SizeBytes: newFile.size, ModifiedAt: newFile.modTime})
		default:
			result.Unchanged++
		}
	}
	changed := result.Added + result.Deleted + result.Modified
	result.ReviewRecommended = result.Deleted >= 50 || (result.SnapshotFiles >= 20 && changed >= 20 && float64(changed)/float64(result.SnapshotFiles) >= 0.25)
	return result, nil
}

func (diff *SnapshotDiff) addSample(change SnapshotChange) {
	if len(diff.Changes) < maxSnapshotDiffSamples {
		diff.Changes = append(diff.Changes, change)
	}
}

func scanDiffTree(root string) (map[string]diffFile, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, errors.New("snapshot comparison root is unavailable")
	}
	files := make(map[string]diffFile)
	err = filepath.WalkDir(resolved, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		if len(files) >= maxSnapshotDiffFiles {
			return errors.New("snapshot comparison reached the 250,000 file safety limit")
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(resolved, current)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = diffFile{size: fileInfo.Size(), modTime: fileInfo.ModTime().UTC()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
