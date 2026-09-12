package files

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOperationsStayInsideShareRootAndUseRecycleBin(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Documents"), 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Documents", "notes.txt"), []byte("hello"), 0o660); err != nil {
		t.Fatal(err)
	}
	entries, err := List(root, "/Documents")
	if err != nil || len(entries) != 1 || entries[0].Name != "notes.txt" {
		t.Fatalf("unexpected listing: %#v err=%v", entries, err)
	}
	if err := MakeDir(root, "/Documents", "Archive"); err != nil {
		t.Fatal(err)
	}
	if err := Rename(root, "/Documents", "notes.txt", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	deleted, err := Delete(root, "share-1", "/Documents", []string{"renamed.txt"})
	if err != nil || deleted != 1 {
		t.Fatalf("delete failed: %d %v", deleted, err)
	}
	trash, err := ListTrash(root, "share-1")
	if err != nil || len(trash) != 1 || trash[0].Name != "renamed.txt" || trash[0].Owner == "" || trash[0].ExpiresAt.IsZero() {
		t.Fatalf("unexpected recycle bin: %#v err=%v", trash, err)
	}
	if ok, err := Restore(root, trash[0].ID); err != nil || !ok {
		t.Fatalf("restore failed: %v %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(root, "Documents", "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "/../outside"); err == nil {
		t.Fatal("path traversal should be rejected")
	}
	if _, err := Resolve(root, "/.mynas-trash"); err == nil {
		t.Fatal("recycle-bin internals should not be addressable")
	}
}

func TestSearchAndUploadStayScopedAndCrossRootMoveCopiesThenDeletes(t *testing.T) {
	sourceRoot := t.TempDir()
	targetRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceRoot, "nested"), 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "nested", "report.txt"), []byte("report"), 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(targetRoot, "inbox"), 0o770); err != nil {
		t.Fatal(err)
	}
	entries, err := Search(sourceRoot, "/", "report")
	if err != nil || len(entries) != 1 || entries[0].Name != "nested/report.txt" {
		t.Fatalf("unexpected search results: %#v %v", entries, err)
	}
	name, err := WriteUpload(targetRoot, "/inbox", "upload.txt", bytes.NewReader([]byte("uploaded")), 1024)
	if err != nil || name != "upload.txt" {
		t.Fatalf("upload failed: %q %v", name, err)
	}
	count, err := Transfer(TransferInput{SourceRoot: sourceRoot, SourcePath: "/nested", Names: []string{"report.txt"}, TargetRoot: targetRoot, TargetPath: "/inbox", Operation: "move"})
	if err != nil || count != 1 {
		t.Fatalf("cross-root move failed: %d %v", count, err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "nested", "report.txt")); !os.IsNotExist(err) {
		t.Fatalf("source was not removed after cross-root move: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(targetRoot, "inbox", "report.txt")); err != nil || string(data) != "report" {
		t.Fatalf("moved data missing: %q %v", data, err)
	}
}

func TestTransferConflictsAndSafeCopy(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "source"), 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "target"), 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "file.txt"), []byte("source"), 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "target", "file.txt"), []byte("target"), 0o660); err != nil {
		t.Fatal(err)
	}
	_, err := Transfer(TransferInput{SourceRoot: root, SourcePath: "/source", Names: []string{"file.txt"}, TargetRoot: root, TargetPath: "/target", Operation: "copy"})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || len(conflict.Names) != 1 {
		t.Fatalf("expected conflict, got %v", err)
	}
	count, err := Transfer(TransferInput{SourceRoot: root, SourcePath: "/source", Names: []string{"file.txt"}, TargetRoot: root, TargetPath: "/target", Operation: "copy", Conflict: "rename"})
	if err != nil || count != 1 {
		t.Fatalf("copy failed: %d %v", count, err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "file (2).txt")); err != nil {
		t.Fatal(err)
	}
}
