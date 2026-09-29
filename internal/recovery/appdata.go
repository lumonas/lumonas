package recovery

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const DefaultAppdataArchiveLimit int64 = 20 << 30

var errAppdataArchiveLimit = errors.New("appdata archive exceeds the configured limit")

type AppdataPayload struct {
	Stack         string
	ContainerPath string
	HostPath      string
	Archive       []byte
}

type DatabaseDumpPayload struct {
	Stack     string
	Container string
	Dump      []byte
}

type DatabaseDumpRecord struct {
	Stack        string `json:"stack"`
	Container    string `json:"container"`
	ArchivePath  string `json:"archivePath"`
	ArchiveBytes int64  `json:"archiveBytes"`
}

func databaseDumpPath(stack, container string) string {
	return "docker/database-dumps/" + stack + "/" + container + ".dump"
}

func validateDatabaseDump(payload DatabaseDumpPayload) error {
	if !safeName(payload.Stack) || strings.Contains(payload.Stack, "/") {
		return errors.New("database dump stack name is invalid")
	}
	if !safeName(payload.Container) || strings.Contains(payload.Container, "/") {
		return errors.New("database dump container name is invalid")
	}
	if len(payload.Dump) == 0 || int64(len(payload.Dump)) > MaxBundleEntryBytes {
		return errors.New("database dump is empty or exceeds the bundle entry limit")
	}
	return nil
}

func parseDatabaseDumpManifest(raw []byte, files map[string][]byte) ([]DatabaseDumpRecord, error) {
	var records []DatabaseDumpRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("database dump manifest is invalid: %w", err)
	}
	seen := make(map[string]bool, len(records))
	for index := range records {
		record := &records[index]
		if err := validateDatabaseDump(DatabaseDumpPayload{Stack: record.Stack, Container: record.Container, Dump: files[record.ArchivePath]}); err != nil {
			return nil, fmt.Errorf("database dump manifest entry %d: %w", index, err)
		}
		if record.ArchivePath != databaseDumpPath(record.Stack, record.Container) || seen[record.ArchivePath] {
			return nil, fmt.Errorf("database dump manifest entry %d has an invalid or duplicate archive path", index)
		}
		if record.ArchiveBytes != int64(len(files[record.ArchivePath])) {
			return nil, fmt.Errorf("database dump manifest entry %d has an incorrect size", index)
		}
		seen[record.ArchivePath] = true
	}
	return records, nil
}

type AppdataRecord struct {
	Stack         string `json:"stack"`
	ContainerPath string `json:"containerPath"`
	HostPath      string `json:"hostPath"`
	ArchivePath   string `json:"archivePath"`
	ArchiveBytes  int64  `json:"archiveBytes"`
}

type SharePayload struct {
	ID      string
	Name    string
	Path    string
	Archive []byte
}

type ShareRecord struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	ArchivePath  string `json:"archivePath"`
	ArchiveBytes int64  `json:"archiveBytes"`
}

const DefaultShareArchiveLimit int64 = 20 << 30

func shareArchivePath(id string) string {
	digest := sha256.Sum256([]byte(id))
	return "shares/data/" + hex.EncodeToString(digest[:16]) + ".tar.gz.enc"
}

func validateSharePayload(payload SharePayload) error {
	if !safeName(payload.ID) || strings.Contains(payload.ID, "/") {
		return errors.New("share id is invalid")
	}
	if strings.TrimSpace(payload.Name) == "" || strings.ContainsRune(payload.Name, '\x00') {
		return errors.New("share name is invalid")
	}
	if !validSharePath(payload.Path) {
		return errors.New("share path is not approved")
	}
	if len(payload.Archive) == 0 {
		return errors.New("share archive is empty")
	}
	if err := validateAppdataArchive(payload.Archive, DefaultShareArchiveLimit); err != nil {
		return fmt.Errorf("share archive is invalid: %w", err)
	}
	return nil
}

func validSharePath(value string) bool {
	if value == "" || !filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return false
	}
	clean := filepath.Clean(value)
	if clean != value || clean == "/" {
		return false
	}
	for _, blocked := range []string{"/proc", "/sys", "/dev", "/etc", "/boot", "/usr", "/bin", "/sbin", "/root", "/run/lumonas", "/var/run/lumonas", "/var/lib/lumonas", "/etc/lumonas", "/srv/lumonas"} {
		if clean == blocked || strings.HasPrefix(clean, blocked+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

func sharePathsOverlap(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	leftToRight, leftErr := filepath.Rel(left, right)
	rightToLeft, rightErr := filepath.Rel(right, left)
	return leftErr == nil && (leftToRight == "." || (leftToRight != ".." && !strings.HasPrefix(leftToRight, ".."+string(filepath.Separator)))) || rightErr == nil && (rightToLeft == "." || (rightToLeft != ".." && !strings.HasPrefix(rightToLeft, ".."+string(filepath.Separator))))
}

func parseShareManifest(raw []byte, files map[string][]byte, key []byte) ([]ShareRecord, error) {
	var records []ShareRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("share manifest is invalid: %w", err)
	}
	seen := make(map[string]bool, len(records))
	for index := range records {
		record := &records[index]
		if !safeName(record.ID) || strings.Contains(record.ID, "/") || strings.TrimSpace(record.Name) == "" || !validSharePath(record.Path) {
			return nil, fmt.Errorf("share manifest entry %d is invalid", index)
		}
		if record.ArchivePath != shareArchivePath(record.ID) || seen[record.ID] {
			return nil, fmt.Errorf("share manifest entry %d has an invalid or duplicate archive path", index)
		}
		for previous := 0; previous < index; previous++ {
			if sharePathsOverlap(records[previous].Path, record.Path) {
				return nil, fmt.Errorf("share manifest paths overlap: %q and %q", records[previous].Path, record.Path)
			}
		}
		seen[record.ID] = true
		ciphertext, ok := files[record.ArchivePath]
		if !ok {
			return nil, fmt.Errorf("share archive is missing: %s", record.ArchivePath)
		}
		plaintext, err := decrypt(ciphertext, key)
		if err != nil {
			return nil, fmt.Errorf("share archive decryption failed for %q: %w", record.Name, err)
		}
		if record.ArchiveBytes != int64(len(plaintext)) {
			return nil, fmt.Errorf("share archive size mismatch for %q", record.Name)
		}
		if err := validateAppdataArchive(plaintext, DefaultShareArchiveLimit); err != nil {
			return nil, fmt.Errorf("share archive %q is invalid: %w", record.Name, err)
		}
	}
	return records, nil
}

func ArchiveAppdata(root string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultAppdataArchiveLimit
	}
	root = filepath.Clean(root)
	if !validArchiveRoot(root) {
		return nil, errors.New("appdata source must be a safe absolute path")
	}
	if _, err := os.Lstat(root); err != nil {
		return nil, err
	}
	var buffer limitedBuffer
	buffer.limit = maxBytes
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	directories := make(map[string]os.FileInfo)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("appdata archive refuses symlink: %s", path)
		}
		if info.IsDir() {
			directories[path] = info
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		name := filepath.ToSlash(relative)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		header.Mode = int64(info.Mode().Perm())
		switch {
		case info.IsDir():
			header.Typeflag = tar.TypeDir
		case info.Mode().IsRegular():
			header.Typeflag = tar.TypeReg
		default:
			return fmt.Errorf("unsupported appdata entry type: %s", path)
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		openedInfo, err := file.Stat()
		if err != nil || !sameArchivedFile(info, openedInfo) {
			_ = file.Close()
			return fmt.Errorf("appdata file changed while it was being archived: %s", path)
		}
		_, copyErr := io.Copy(tarWriter, file)
		finishedInfo, statErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil {
			return statErr
		}
		if !sameArchivedFile(info, finishedInfo) {
			return fmt.Errorf("appdata file changed while it was being archived: %s", path)
		}
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err == nil {
		for directory, before := range directories {
			after, statErr := os.Stat(directory)
			if statErr != nil || !sameArchivedFile(before, after) {
				err = fmt.Errorf("appdata directory changed while it was being archived: %s", directory)
				break
			}
		}
	}
	closeTarErr := tarWriter.Close()
	closeGzipErr := gzipWriter.Close()
	if err != nil {
		return nil, err
	}
	if closeTarErr != nil {
		return nil, closeTarErr
	}
	if closeGzipErr != nil {
		return nil, closeGzipErr
	}
	return buffer.Bytes(), nil
}

func sameArchivedFile(before, after os.FileInfo) bool {
	return before != nil && after != nil && os.SameFile(before, after) && before.Size() == after.Size() && before.Mode() == after.Mode() && before.ModTime().Equal(after.ModTime())
}

func ExtractAppdata(data []byte, destination string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultAppdataArchiveLimit
	}
	destination = filepath.Clean(destination)
	if !validArchiveRoot(destination) {
		return errors.New("appdata destination must be a safe absolute path")
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	var extracted int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !safeArchiveMember(header.Name) {
			return fmt.Errorf("unsafe appdata archive member %q", header.Name)
		}
		target := filepath.Join(destination, filepath.FromSlash(header.Name))
		if !withinRoot(destination, target) {
			return fmt.Errorf("appdata archive member escapes destination: %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := rejectSymlinkPath(destination, target); err != nil {
				return err
			}
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size < 0 || extracted > maxBytes-header.Size {
				return errAppdataArchiveLimit
			}
			if err := rejectSymlinkPath(destination, filepath.Dir(target)); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			temporary, err := os.CreateTemp(filepath.Dir(target), ".lumonas-appdata-*")
			if err != nil {
				return err
			}
			temporaryPath := temporary.Name()
			cleanup := func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }
			if err := temporary.Chmod(os.FileMode(header.Mode).Perm()); err != nil {
				cleanup()
				return err
			}
			if _, err := io.CopyN(temporary, tarReader, header.Size); err != nil {
				cleanup()
				return err
			}
			if err := temporary.Sync(); err != nil {
				cleanup()
				return err
			}
			if err := temporary.Close(); err != nil {
				_ = os.Remove(temporaryPath)
				return err
			}
			if err := os.Rename(temporaryPath, target); err != nil {
				_ = os.Remove(temporaryPath)
				return err
			}
			extracted += header.Size
		default:
			return fmt.Errorf("unsupported appdata archive member type %q", header.Name)
		}
	}
	return nil
}

func validateAppdataArchive(data []byte, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultAppdataArchiveLimit
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	seen := map[string]bool{}
	var expanded int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !safeArchiveMember(header.Name) {
			return fmt.Errorf("unsafe archive member %q", header.Name)
		}
		if seen[header.Name] {
			return fmt.Errorf("duplicate archive member %q", header.Name)
		}
		seen[header.Name] = true
		switch header.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg:
			if header.Size < 0 || expanded > maxBytes-header.Size {
				return errAppdataArchiveLimit
			}
			expanded += header.Size
			if _, err := io.CopyN(io.Discard, tarReader, header.Size); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive member type %q", header.Name)
		}
	}
	return nil
}

func appdataArchivePath(stack, containerPath string) string {
	digest := sha256.Sum256([]byte(stack + "\x00" + containerPath))
	return "docker/appdata/" + stack + "/" + hex.EncodeToString(digest[:8]) + ".tar.gz"
}

func validateAppdataPayload(payload AppdataPayload) error {
	if !safeName(payload.Stack) {
		return errors.New("appdata stack name is invalid")
	}
	if !validContainerPath(payload.ContainerPath) {
		return errors.New("appdata container path is invalid")
	}
	if !validAppdataHostPath(payload.HostPath) {
		return errors.New("appdata host path is not approved")
	}
	if len(payload.Archive) == 0 {
		return errors.New("appdata archive is empty")
	}
	return nil
}

func parseAppdataManifest(raw []byte, files map[string][]byte) ([]AppdataRecord, error) {
	var records []AppdataRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("appdata manifest is invalid: %w", err)
	}
	seen := make(map[string]bool, len(records))
	for index := range records {
		record := &records[index]
		if err := validateAppdataPayload(AppdataPayload{Stack: record.Stack, ContainerPath: record.ContainerPath, HostPath: record.HostPath, Archive: files[record.ArchivePath]}); err != nil {
			return nil, fmt.Errorf("appdata manifest entry %d: %w", index, err)
		}
		if record.ArchivePath != appdataArchivePath(record.Stack, record.ContainerPath) {
			return nil, fmt.Errorf("appdata manifest entry %d has an invalid archive path", index)
		}
		if seen[record.ArchivePath] {
			return nil, fmt.Errorf("duplicate appdata manifest entry %q", record.ArchivePath)
		}
		seen[record.ArchivePath] = true
		if record.ArchiveBytes != int64(len(files[record.ArchivePath])) {
			return nil, fmt.Errorf("appdata archive size mismatch for %q", record.ArchivePath)
		}
	}
	return records, nil
}

func validContainerPath(value string) bool {
	if value == "" || !filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return false
	}
	clean := filepath.Clean(value)
	return clean == value && clean != "/" && !strings.Contains(clean, "..")
}

func validAppdataHostPath(value string) bool {
	if value == "" || !filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return false
	}
	clean := filepath.Clean(value)
	if clean == "/" || strings.Contains(clean, "..") {
		return false
	}
	for _, prefix := range []string{"/srv/", "/mnt/", "/opt/", "/var/lib/"} {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

func validArchiveRoot(value string) bool {
	if value == "" || !filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return false
	}
	clean := filepath.Clean(value)
	return clean != "/" && !strings.Contains(clean, "..")
}

func safeArchiveMember(name string) bool {
	return safeName(filepath.ToSlash(name))
}

type limitedBuffer struct {
	data  []byte
	limit int64
}

func (w *limitedBuffer) Write(data []byte) (int, error) {
	if int64(len(w.data))+int64(len(data)) > w.limit {
		return 0, errAppdataArchiveLimit
	}
	w.data = append(w.data, data...)
	return len(data), nil
}

func (w *limitedBuffer) Bytes() []byte { return w.data }
