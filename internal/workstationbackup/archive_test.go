package workstationbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEncryptedArchiveRoundTripAndWrongKey(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	contents := bytes.Repeat([]byte("private workstation data\n"), 800)
	if err := os.WriteFile(filepath.Join(root, "nested", "notes.txt"), contents, 0o640); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "backup.lwb")
	if err := createEncryptedArchive(root, archivePath, key); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(archivePath); err != nil || bytes.Contains(data, []byte("private workstation data")) {
		t.Fatalf("archive did not conceal source bytes: err=%v", err)
	}
	badKey := append([]byte(nil), key...)
	badKey[0] ^= 0xff
	if err := extractArchive(archivePath, filepath.Join(t.TempDir(), "bad"), badKey, false); err == nil {
		t.Fatal("wrong key unexpectedly restored the archive")
	}
	destination := filepath.Join(t.TempDir(), "restore")
	if err := extractArchive(archivePath, destination, key, false); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(destination, "nested", "notes.txt"))
	if err != nil || !bytes.Equal(actual, contents) {
		t.Fatalf("restored content differs: err=%v", err)
	}
}

func TestEncryptedArchiveRespectsSelectiveIncludeAndExcludePatterns(t *testing.T) {
	root := t.TempDir()
	for name, contents := range map[string]string{
		"Documents/notes.txt":           "keep this",
		"Documents/private/secrets.txt": "exclude this folder",
		"Documents/cache.tmp":           "exclude this file",
		"Photos/family.jpg":             "not selected",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "selected.lwb")
	selection := Selection{Include: []string{"Documents"}, Exclude: []string{"Documents/private", "*.tmp"}}
	if err := createEncryptedArchiveSelected(root, archivePath, key, selection); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "restored")
	if err := extractArchive(archivePath, destination, key, false); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "Documents", "notes.txt")); err != nil || string(data) != "keep this" {
		t.Fatalf("selected file was missing or changed: %q %v", data, err)
	}
	for _, omitted := range []string{"Documents/private/secrets.txt", "Documents/cache.tmp", "Photos/family.jpg"} {
		if _, err := os.Lstat(filepath.Join(destination, filepath.FromSlash(omitted))); !os.IsNotExist(err) {
			t.Fatalf("excluded file %q was restored, err=%v", omitted, err)
		}
	}
}

func TestSelectionRejectsPathsOutsideTheSourceRoot(t *testing.T) {
	for _, selection := range []Selection{
		{Include: []string{"../private"}},
		{Exclude: []string{"/private"}},
		{Include: []string{"[invalid"}},
	} {
		if err := selection.Validate(); err == nil {
			t.Fatalf("unsafe selection was accepted: %#v", selection)
		}
	}
}

func TestEncryptChunksRejectsTamperingAndTruncation(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	var encrypted bytes.Buffer
	if err := encryptChunks(strings.NewReader("authenticated data"), &encrypted, key); err != nil {
		t.Fatal(err)
	}
	mutated := append([]byte(nil), encrypted.Bytes()...)
	mutated[len(mutated)-1] ^= 1
	if err := decryptChunks(bytes.NewReader(mutated), io.Discard, key); err == nil {
		t.Fatal("tampered encrypted chunk was accepted")
	}
	if err := decryptChunks(bytes.NewReader(encrypted.Bytes()[:len(encrypted.Bytes())-1]), io.Discard, key); err == nil {
		t.Fatal("truncated encrypted chunk was accepted")
	}
}

func TestSafeArchiveTargetAndRestoreRejectSymlinkParents(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../outside", "/absolute", `C:\outside`, `nested\\escape`} {
		if _, err := safeArchiveTarget(root, name); err == nil {
			t.Errorf("unsafe path %q accepted", name)
		}
	}
	if runtime.GOOS != "windows" {
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
			t.Fatal(err)
		}
		if err := ensureSafeDirectory(root, filepath.Join(root, "linked", "child")); err == nil {
			t.Fatal("restore followed an existing symlink in the destination")
		}
	}
}

func TestBackupSkipsSymlinksAndRejectsSymlinkSourceRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires additional Windows privileges")
	}
	source := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	archivePath := filepath.Join(t.TempDir(), "backup.lwb")
	if err := createEncryptedArchive(source, archivePath, key); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := extractArchive(archivePath, destination, key, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(destination, "linked.txt")); !os.IsNotExist(err) {
		t.Fatalf("backup unexpectedly included a symlink: %v", err)
	}
	rootLink := filepath.Join(t.TempDir(), "source-link")
	if err := os.Symlink(source, rootLink); err != nil {
		t.Fatal(err)
	}
	if err := createEncryptedArchive(rootLink, filepath.Join(t.TempDir(), "linked-root.lwb"), key); err == nil {
		t.Fatal("backup followed a symlink used as the source root")
	}
}

func TestExtractArchiveRefusesOverwriteByDefault(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	archivePath := filepath.Join(t.TempDir(), "backup.lwb")
	if err := createEncryptedArchive(root, archivePath, key); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := os.WriteFile(filepath.Join(destination, "note.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractArchive(archivePath, destination, key, false); err == nil {
		t.Fatal("restore silently overwrote an existing file")
	}
	data, err := os.ReadFile(filepath.Join(destination, "note.txt"))
	if err != nil || string(data) != "keep" {
		t.Fatalf("existing destination content changed: %q err=%v", data, err)
	}
}

func TestExtractArchiveRejectsAuthenticatedTraversalEntry(t *testing.T) {
	plain := &bytes.Buffer{}
	compressed := gzip.NewWriter(plain)
	archive := tar.NewWriter(compressed)
	if err := archive.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0o600, Size: 4, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = archive.Write([]byte("evil"))
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	var encrypted bytes.Buffer
	if err := encryptChunks(bytes.NewReader(plain.Bytes()), &encrypted, key); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "traversal.lwb")
	if err := os.WriteFile(archivePath, encrypted.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := extractArchive(archivePath, destination, key, false); err == nil {
		t.Fatal("authenticated archive traversal path was extracted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destination), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive wrote outside destination: %v", err)
	}
}

func TestBackupKeyRoundTripFromBase64(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !bytes.Equal(decoded, key) {
		t.Fatalf("backup key encoding failed: %v", err)
	}
}
