package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const maxIntegrityFiles = 200000

type IntegrityFile struct {
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

// BuildIntegrityManifest hashes regular files below a managed root. Symlinks
// and special files are excluded; the file-count ceiling bounds metadata use.
func BuildIntegrityManifest(root string) ([]IntegrityFile, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	manifest := make([]IntegrityFile, 0)
	err = filepath.WalkDir(resolved, func(current string, entry os.DirEntry, walkErr error) error {
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
		if len(manifest) >= maxIntegrityFiles {
			return errors.New("integrity scan reached the 200,000 file safety limit")
		}
		walkInfo, err := entry.Info()
		if err != nil {
			return err
		}
		file, err := os.Open(current)
		if err != nil {
			return err
		}
		openInfo, statErr := file.Stat()
		if statErr != nil || !openInfo.Mode().IsRegular() || !os.SameFile(walkInfo, openInfo) {
			_ = file.Close()
			return errors.New("file changed during integrity scan")
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		relative, err := filepath.Rel(resolved, current)
		if err != nil {
			return err
		}
		manifest = append(manifest, IntegrityFile{Path: filepath.ToSlash(relative), SHA256: hex.EncodeToString(hash.Sum(nil)), SizeBytes: openInfo.Size(), ModifiedAt: openInfo.ModTime().UTC()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Path < manifest[j].Path })
	return manifest, nil
}
