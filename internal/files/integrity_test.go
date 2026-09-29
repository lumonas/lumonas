package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildIntegrityManifestHashesFilesAndSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "document.txt"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildIntegrityManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 1 || manifest[0].Path != "document.txt" || manifest[0].SHA256 == "" || manifest[0].SizeBytes != 5 {
		t.Fatalf("unexpected integrity manifest: %#v", manifest)
	}
}
