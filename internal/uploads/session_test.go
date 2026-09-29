package uploads

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResumableUploadPersistsChunksAndCompletesWithChecksum(t *testing.T) {
	root := t.TempDir()
	shareRoot := filepath.Join(root, "share")
	if err := os.MkdirAll(filepath.Join(shareRoot, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	content := []byte("resumable file payload")
	digest := sha256.Sum256(content)
	manager := Manager{Root: filepath.Join(root, "sessions")}
	session, err := manager.Create("share-1", shareRoot, "docs", "payload.bin", int64(len(content)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	first, duplicate, err := manager.WriteChunk(session.ID, 0, bytes.NewReader(content[:8]))
	if err != nil || duplicate || first.ReceivedBytes != 8 {
		t.Fatalf("first chunk = %#v duplicate=%t err=%v", first, duplicate, err)
	}
	resumed, err := (Manager{Root: manager.Root}).Get(session.ID)
	if err != nil || resumed.ReceivedBytes != 8 {
		t.Fatalf("resumed session = %#v err=%v", resumed, err)
	}
	if _, duplicate, err := manager.WriteChunk(session.ID, 0, bytes.NewReader(content[:8])); err != nil || !duplicate {
		t.Fatalf("identical chunk retry was not idempotent: duplicate=%t err=%v", duplicate, err)
	}
	if _, _, err := manager.WriteChunk(session.ID, 7, bytes.NewReader(content[8:])); err == nil {
		t.Fatal("out-of-order chunk was accepted")
	}
	second, _, err := manager.WriteChunk(session.ID, 8, bytes.NewReader(content[8:]))
	if err != nil || second.ReceivedBytes != int64(len(content)) {
		t.Fatalf("second chunk = %#v err=%v", second, err)
	}
	completed, name, err := manager.Complete(session.ID)
	if err != nil || name != "payload.bin" || completed.State != "completed" {
		t.Fatalf("complete result = %#v name=%q err=%v", completed, name, err)
	}
	_, retriedName, err := manager.Complete(session.ID)
	if err != nil || retriedName != name {
		t.Fatalf("completion retry name=%q err=%v", retriedName, err)
	}
	got, err := os.ReadFile(filepath.Join(shareRoot, "docs", name))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("completed file = %q err=%v", got, err)
	}
}

func TestResumableUploadRejectsBadMetadataAndChecksum(t *testing.T) {
	root := t.TempDir()
	shareRoot := filepath.Join(root, "share")
	if err := os.Mkdir(shareRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Root: filepath.Join(root, "sessions")}
	if _, err := manager.Create("share", shareRoot, ".", "../bad", 1, ""); err == nil {
		t.Fatal("invalid name was accepted")
	}
	session, err := manager.Create("share", shareRoot, ".", "file.bin", 3, hex.EncodeToString(make([]byte, sha256.Size)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.WriteChunk(session.ID, 0, bytes.NewReader([]byte("too long"))); err == nil {
		t.Fatal("chunk larger than the remaining length was accepted")
	}
	if _, _, err := manager.WriteChunk(session.ID, 0, bytes.NewReader([]byte("bad"))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Complete(session.ID); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
}

func TestResumableUploadReplacementUsesCompareAndSwapMetadata(t *testing.T) {
	root := t.TempDir()
	shareRoot := filepath.Join(root, "share")
	if err := os.Mkdir(shareRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(shareRoot, "document.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Root: filepath.Join(root, "sessions")}
	content := []byte("updated")
	digest := sha256.Sum256(content)
	options := CreateOptions{ReplaceExisting: true, ExpectedSize: info.Size(), ExpectedModTime: info.ModTime()}
	session, err := manager.CreateWithOptions("share", shareRoot, ".", "document.txt", int64(len(content)), hex.EncodeToString(digest[:]), options)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.WriteChunk(session.ID, 0, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	changed := info.ModTime().Add(2 * time.Second)
	if err := os.WriteFile(target, []byte("concurrent edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, changed, changed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Complete(session.ID); err == nil {
		t.Fatal("replacement overwrote a file changed after preview")
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != "concurrent edit" {
		t.Fatalf("concurrent edit was lost: %q %v", actual, err)
	}

	info, err = os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	options.ExpectedSize, options.ExpectedModTime = info.Size(), info.ModTime()
	session, err = manager.CreateWithOptions("share", shareRoot, ".", "document.txt", int64(len(content)), hex.EncodeToString(digest[:]), options)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.WriteChunk(session.ID, 0, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Complete(session.ID); err != nil {
		t.Fatalf("matching reviewed file was not replaced: %v", err)
	}
	actual, err = os.ReadFile(target)
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("replacement contents = %q, %v", actual, err)
	}
}
