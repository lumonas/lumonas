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

type AppdataRecord struct {
	Stack         string `json:"stack"`
	ContainerPath string `json:"containerPath"`
	HostPath      string `json:"hostPath"`
	ArchivePath   string `json:"archivePath"`
	ArchiveBytes  int64  `json:"archiveBytes"`
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
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
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
