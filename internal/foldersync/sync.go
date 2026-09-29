// Package foldersync plans and applies safe one-way directory synchronization.
package foldersync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Hash    string    `json:"hash,omitempty"`
}

type Change struct {
	Path        string `json:"path"`
	Action      string `json:"action"` // copy, update, delete, archive
	Bytes       int64  `json:"bytes"`
	Hash        string `json:"hash,omitempty"`
	Reason      string `json:"reason,omitempty"`
	From        string `json:"from,omitempty"` // left or right for two-way runs
	To          string `json:"to,omitempty"`
	ArchivePath string `json:"archivePath,omitempty"`
}

type Plan struct {
	Changes   []Change `json:"changes"`
	Files     int      `json:"files"`
	Bytes     int64    `json:"bytes"`
	Deletes   int      `json:"deletes"`
	Conflicts int      `json:"conflicts,omitempty"`
}

// Scan indexes regular files under root. Symlinks are always skipped.
func Scan(root string, deep bool) (map[string]Entry, error) {
	return ScanFiltered(root, deep, nil)
}

// ScanFiltered indexes regular files under root while excluding configured
// relative glob patterns. Symlinks and LumoNAS's managed version directory
// are always skipped.
func ScanFiltered(root string, deep bool, ignorePatterns []string) (map[string]Entry, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("sync endpoint must be a directory")
	}
	var scoped *os.Root
	if deep {
		scoped, err = os.OpenRoot(root)
		if err != nil {
			return nil, err
		}
		defer scoped.Close()
	}
	result := map[string]Entry{}
	err = filepath.WalkDir(root, func(current string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.Type()&os.ModeSymlink != 0 {
			if item.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if item.IsDir() {
			if current != root && (item.Name() == ".lumonas-versions" || item.Name() == ".lumonas-conflicts") {
				return filepath.SkipDir
			}
			if current != root {
				relative, relErr := filepath.Rel(root, current)
				if relErr != nil {
					return relErr
				}
				if matchesIgnorePattern(filepath.ToSlash(relative), ignorePatterns) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !item.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") {
			return errors.New("unsafe path found in sync root")
		}
		if matchesIgnorePattern(rel, ignorePatterns) {
			return nil
		}
		stat, err := item.Info()
		if err != nil {
			return err
		}
		entry := Entry{Path: rel, Size: stat.Size(), ModTime: stat.ModTime().UTC()}
		if deep {
			file, err := scoped.Open(filepath.FromSlash(rel))
			if err != nil {
				return err
			}
			openedInfo, statErr := file.Stat()
			if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(stat, openedInfo) {
				_ = file.Close()
				return errors.New("sync file changed while the share was being scanned")
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
			entry.Hash = hex.EncodeToString(hash.Sum(nil))
		}
		result[rel] = entry
		return nil
	})
	return result, err
}

func matchesIgnorePattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlobSegments(strings.Split(pattern, "/"), strings.Split(name, "/")) {
			return true
		}
	}
	return false
}

// FilterEntries applies the same relative ignore rules to a remote listing.
func FilterEntries(entries map[string]Entry, patterns []string) map[string]Entry {
	if len(patterns) == 0 {
		return entries
	}
	filtered := make(map[string]Entry, len(entries))
	for name, entry := range entries {
		if !matchesIgnorePattern(name, patterns) {
			filtered[name] = entry
		}
	}
	return filtered
}

func matchGlobSegments(pattern, value []string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		if matchGlobSegments(pattern[1:], value) {
			return true
		}
		return len(value) > 0 && matchGlobSegments(pattern, value[1:])
	}
	if len(value) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], value[0])
	return err == nil && matched && matchGlobSegments(pattern[1:], value[1:])
}

func BuildPlan(source, destination map[string]Entry, mirror, deep bool) Plan {
	plan := Plan{Changes: []Change{}}
	paths := make([]string, 0, len(source))
	for name := range source {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		from := source[name]
		to, exists := destination[name]
		if !exists {
			plan.Changes = append(plan.Changes, Change{Path: name, Action: "copy", Bytes: from.Size, Hash: from.Hash})
			plan.Files++
			plan.Bytes += from.Size
		} else if from.Size != to.Size || !from.ModTime.Equal(to.ModTime) || (deep && from.Hash != to.Hash) {
			plan.Changes = append(plan.Changes, Change{Path: name, Action: "update", Bytes: from.Size, Hash: from.Hash})
			plan.Files++
			plan.Bytes += from.Size
		}
	}
	if mirror {
		paths = paths[:0]
		for name := range destination {
			if _, ok := source[name]; !ok {
				paths = append(paths, name)
			}
		}
		sort.Strings(paths)
		for _, name := range paths {
			plan.Changes = append(plan.Changes, Change{Path: name, Action: "delete", Bytes: destination[name].Size})
			plan.Deletes++
		}
	}
	return plan
}

// Apply copies changed files atomically, verifies each copied file, and only
// performs mirror deletions after all requested copies succeeded.
func Apply(ctx context.Context, sourceRoot, destinationRoot string, plan Plan, progress func(done int, bytes int64)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeRoots(sourceRoot, destinationRoot); err != nil {
		return err
	}
	source, err := os.OpenRoot(sourceRoot)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenRoot(destinationRoot)
	if err != nil {
		return err
	}
	defer destination.Close()
	var done int
	var transferred int64
	for _, change := range plan.Changes {
		if change.Action == "delete" {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		relative, err := cleanRelativePath(change.Path)
		if err != nil {
			return err
		}
		src, err := source.Open(relative)
		if err != nil {
			return err
		}
		info, err := src.Stat()
		if err != nil || !info.Mode().IsRegular() {
			_ = src.Close()
			return errors.New("sync source is no longer a regular file")
		}
		if err := copyVerifiedIntoRoot(ctx, destination, relative, src); err != nil {
			_ = src.Close()
			return fmt.Errorf("%s: %w", change.Path, err)
		}
		if err := src.Close(); err != nil {
			return err
		}
		done++
		transferred += change.Bytes
		if progress != nil {
			progress(done, transferred)
		}
	}
	for _, change := range plan.Changes {
		if change.Action != "delete" {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		relative, err := cleanRelativePath(change.Path)
		if err != nil {
			return err
		}
		if err := destination.Remove(relative); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete %s: %w", change.Path, err)
		}
	}
	return nil
}

// ApplyPull downloads planned files through get into atomic local replacements.
func ApplyPull(ctx context.Context, destinationRoot string, plan Plan, get func(context.Context, string, string) error, progress func(int, int64)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	root, err := filepath.EvalSymlinks(destinationRoot)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return errors.New("sync destination is unavailable")
	}
	destination, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer destination.Close()
	var done int
	var transferred int64
	for _, change := range plan.Changes {
		if change.Action == "delete" {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		relative, err := cleanRelativePath(change.Path)
		if err != nil {
			return err
		}
		temporary, err := os.CreateTemp("", ".lumonas-sync-pull-*.tmp")
		if err != nil {
			return err
		}
		temporaryPath := temporary.Name()
		if err := temporary.Close(); err != nil {
			os.Remove(temporaryPath)
			return err
		}
		if err := get(ctx, change.Path, temporaryPath); err != nil {
			os.Remove(temporaryPath)
			return fmt.Errorf("download %s: %w", change.Path, err)
		}
		digest, size, err := fileDigest(temporaryPath)
		if err != nil {
			os.Remove(temporaryPath)
			return err
		}
		if size != change.Bytes || (change.Hash != "" && !strings.EqualFold(digest, change.Hash)) {
			os.Remove(temporaryPath)
			return fmt.Errorf("download verification mismatch for %s", change.Path)
		}
		input, err := os.Open(temporaryPath)
		if err != nil {
			os.Remove(temporaryPath)
			return err
		}
		if err := copyVerifiedIntoRoot(ctx, destination, relative, input); err != nil {
			_ = input.Close()
			os.Remove(temporaryPath)
			return err
		}
		if err := input.Close(); err != nil {
			os.Remove(temporaryPath)
			return err
		}
		_ = os.Remove(temporaryPath)
		done++
		transferred += size
		if progress != nil {
			progress(done, transferred)
		}
	}
	for _, change := range plan.Changes {
		if change.Action != "delete" {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		relative, err := cleanRelativePath(change.Path)
		if err != nil {
			return err
		}
		if err := destination.Remove(relative); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete %s: %w", change.Path, err)
		}
	}
	return nil
}

func safeRoots(source, destination string) error {
	src, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	dstAbsolute, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if filepath.Clean(src) == filepath.Clean(dstAbsolute) || within(src, dstAbsolute) || within(dstAbsolute, src) {
		return errors.New("sync endpoints cannot contain one another")
	}
	dst, err := filepath.EvalSymlinks(destination)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(destination, 0o750); err != nil {
			return err
		}
		dst, err = filepath.EvalSymlinks(destination)
		if err != nil {
			return err
		}
	}
	if filepath.Clean(src) == filepath.Clean(dst) {
		return errors.New("sync source and destination must differ")
	}
	if within(src, dst) || within(dst, src) {
		return errors.New("sync endpoints cannot contain one another")
	}
	return nil
}

// CheckRoots validates that managed sync roots are distinct and do not nest.
func CheckRoots(source, destination string) error { return safeRoots(source, destination) }

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func safeFile(root, name string) (string, error) {
	name, err := cleanRelativePath(name)
	if err != nil {
		return "", err
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(base, filepath.FromSlash(name))
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("sync path escapes endpoint")
	}
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("sync path contains a symbolic link")
		}
	}
	return target, nil
}

func cleanRelativePath(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\\x00\r\n") || filepath.IsAbs(name) {
		return "", errors.New("sync path is unsafe")
	}
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("sync path is unsafe")
		}
	}
	return filepath.FromSlash(name), nil
}

func copyVerifiedIntoRoot(ctx context.Context, destination *os.Root, relative string, input io.Reader) error {
	if err := destination.MkdirAll(filepath.Dir(relative), 0o750); err != nil {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(relative), ".lumonas-sync-"+hex.EncodeToString(random[:])+".tmp")
	output, err := destination.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer destination.Remove(temporary)
	hashSource := sha256.New()
	hashTarget := sha256.New()
	tee := io.TeeReader(&contextReader{ctx: ctx, reader: input}, hashSource)
	if _, err := io.Copy(output, tee); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	check, err := destination.Open(temporary)
	if err != nil {
		return err
	}
	if _, err := io.Copy(hashTarget, check); err != nil {
		_ = check.Close()
		return err
	}
	if err := check.Close(); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hashSource.Sum(nil)), hex.EncodeToString(hashTarget.Sum(nil))) {
		return errors.New("transfer verification failed")
	}
	if err := destination.Rename(temporary, relative); err != nil {
		return err
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(buffer)
	}
}

func fileDigest(name string) (string, int64, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}
