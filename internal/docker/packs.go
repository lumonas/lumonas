package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const (
	// manifestMaxBytes bounds the pack manifest read into memory.
	manifestMaxBytes = 1 << 20
	// packMaxImageBytes bounds a single image archive inside a pack; it
	// matches the interactive upload limit.
	packMaxImageBytes = 20 << 30
	// packMaxImages bounds how many archives one pack may reference.
	packMaxImages = 256
)

var packNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var packFilePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}\.tar$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ImagePackImage describes one docker save archive inside a pack.
type ImagePackImage struct {
	File       string `json:"file"`
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
	SHA256     string `json:"sha256"`
	SizeBytes  uint64 `json:"sizeBytes,omitempty"`
}

// ImagePack is the manifest of an offline image pack: a directory holding
// this manifest plus the referenced archives.
type ImagePack struct {
	Name        string           `json:"name"`
	Version     string           `json:"version,omitempty"`
	Description string           `json:"description,omitempty"`
	Images      []ImagePackImage `json:"images"`
}

// ImagePackSummary is a pack as listed in the pack repository root.
type ImagePackSummary struct {
	Name           string `json:"name"`
	Version        string `json:"version,omitempty"`
	Description    string `json:"description,omitempty"`
	ImageCount     int    `json:"imageCount"`
	TotalSizeBytes uint64 `json:"totalSizeBytes"`
}

// ImagePackFailure reports one archive that could not be imported.
type ImagePackFailure struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// ImagePackImportResult reports the outcome of importing a pack.
type ImagePackImportResult struct {
	Pack     string             `json:"pack"`
	Imported []string           `json:"imported"`
	Failed   []ImagePackFailure `json:"failed,omitempty"`
}

// LoadImagePack reads and validates a pack manifest from dir. Validation is
// fail-closed: every referenced archive must be a plain file name inside the
// pack directory, exist as a regular file (symlinks are rejected), and carry
// a well-formed SHA-256 digest. Content digests are verified by
// ImportImagePack, not here, so listing packs never hashes multi-gigabyte
// archives.
func LoadImagePack(dir string) (ImagePack, error) {
	if !filepath.IsAbs(dir) {
		return ImagePack{}, errors.New("image pack path must be absolute")
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return ImagePack{}, err
	}
	if !info.Mode().IsRegular() {
		return ImagePack{}, errors.New("pack manifest must be a regular file")
	}
	if info.Size() > manifestMaxBytes {
		return ImagePack{}, fmt.Errorf("pack manifest exceeds %d bytes", manifestMaxBytes)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ImagePack{}, err
	}
	var pack ImagePack
	if err := json.Unmarshal(data, &pack); err != nil {
		return ImagePack{}, fmt.Errorf("pack manifest is not valid JSON: %w", err)
	}
	if !packNamePattern.MatchString(pack.Name) {
		return ImagePack{}, errors.New("pack name must contain lowercase letters, numbers, dots, hyphens, or underscores")
	}
	if len(pack.Images) == 0 || len(pack.Images) > packMaxImages {
		return ImagePack{}, fmt.Errorf("pack must reference 1 to %d images", packMaxImages)
	}
	seen := make(map[string]bool, len(pack.Images))
	for _, image := range pack.Images {
		if !packFilePattern.MatchString(image.File) {
			return ImagePack{}, fmt.Errorf("invalid image archive name %q", image.File)
		}
		if seen[image.File] {
			return ImagePack{}, fmt.Errorf("duplicate image archive %q", image.File)
		}
		seen[image.File] = true
		if !sha256Pattern.MatchString(image.SHA256) {
			return ImagePack{}, fmt.Errorf("image archive %q has no valid sha256 digest", image.File)
		}
		archive := filepath.Join(dir, image.File)
		archiveInfo, statErr := os.Lstat(archive)
		if statErr != nil {
			return ImagePack{}, fmt.Errorf("image archive %q is missing", image.File)
		}
		if !archiveInfo.Mode().IsRegular() {
			return ImagePack{}, fmt.Errorf("image archive %q must be a regular file", image.File)
		}
		if image.SizeBytes > packMaxImageBytes {
			return ImagePack{}, fmt.Errorf("image archive %q exceeds the %d byte limit", image.File, packMaxImageBytes)
		}
	}
	return pack, nil
}

// AvailableImagePacks lists valid packs directly inside the pack root.
// Unreadable or invalid directories are skipped so one broken pack cannot
// hide the rest.
func AvailableImagePacks(root string) ([]ImagePackSummary, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("image pack root must be absolute")
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []ImagePackSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]ImagePackSummary, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pack, loadErr := LoadImagePack(filepath.Join(root, entry.Name()))
		if loadErr != nil {
			continue
		}
		summary := ImagePackSummary{Name: pack.Name, Version: pack.Version, Description: pack.Description, ImageCount: len(pack.Images)}
		for _, image := range pack.Images {
			summary.TotalSizeBytes += image.SizeBytes
		}
		result = append(result, summary)
	}
	return result, nil
}

// ImportImagePack verifies every archive checksum first, then loads each
// image through the controlled import path. A checksum mismatch or missing
// file aborts the import before any image is loaded.
func (s Service) ImportImagePack(ctx context.Context, dir string) (ImagePackImportResult, error) {
	pack, err := LoadImagePack(dir)
	if err != nil {
		return ImagePackImportResult{}, err
	}
	for _, image := range pack.Images {
		if err := verifyPackArchive(dir, image); err != nil {
			return ImagePackImportResult{Pack: pack.Name}, err
		}
	}
	result := ImagePackImportResult{Pack: pack.Name, Imported: make([]string, 0, len(pack.Images))}
	for _, image := range pack.Images {
		if err := s.ImportImage(ctx, filepath.Join(dir, image.File)); err != nil {
			result.Failed = append(result.Failed, ImagePackFailure{File: image.File, Reason: err.Error()})
			continue
		}
		ref := image.Repository + ":" + image.Tag
		if image.Repository == "" {
			ref = image.File
		}
		result.Imported = append(result.Imported, ref)
	}
	return result, nil
}

func verifyPackArchive(dir string, image ImagePackImage) error {
	archive, err := os.Open(filepath.Join(dir, image.File))
	if err != nil {
		return fmt.Errorf("image archive %q could not be opened: %w", image.File, err)
	}
	defer archive.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, io.LimitReader(archive, packMaxImageBytes+1))
	if err != nil {
		return fmt.Errorf("image archive %q could not be read: %w", image.File, err)
	}
	if size > packMaxImageBytes {
		return fmt.Errorf("image archive %q exceeds the %d byte limit", image.File, packMaxImageBytes)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != image.SHA256 {
		return fmt.Errorf("image archive %q failed its sha256 checksum", image.File)
	}
	if image.SizeBytes != 0 && uint64(size) != image.SizeBytes {
		return fmt.Errorf("image archive %q is %d bytes, manifest declares %d", image.File, size, image.SizeBytes)
	}
	return nil
}
