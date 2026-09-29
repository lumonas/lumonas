package recovery

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryBundleEncryptsAndRestoresShareData(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("private share contents"), 0o640); err != nil {
		t.Fatal(err)
	}
	archive, err := ArchiveAppdata(source, DefaultShareArchiveLimit)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("recovery-key")
	bundle, err := Create(Input{
		Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte(`{"hostname":"nas"}`),
		Database: []byte("SQLite format 3\x00"),
		Shares:   []SharePayload{{ID: "share-1", Name: "Documents", Path: "/srv/shares/documents", Archive: archive}},
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bundle, []byte("private share contents")) {
		t.Fatal("share contents were stored in plaintext in the bundle")
	}
	plan, err := Plan(bundle, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shares) != 1 || plan.Shares[0].ID != "share-1" {
		t.Fatalf("unexpected share restore plan: %#v", plan.Shares)
	}
	root := t.TempDir()
	result, err := Apply(bundle, key, ApplyOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SharesRestored) != 1 || result.SharesRestored[0] != "share-1" {
		t.Fatalf("unexpected restored shares: %#v", result.SharesRestored)
	}
	data, err := os.ReadFile(filepath.Join(root, "srv/shares/documents/hello.txt"))
	if err != nil || string(data) != "private share contents" {
		t.Fatalf("restored share data = %q, err=%v", data, err)
	}
}

func TestRecoveryBundleRejectsInvalidShareAndWrongKey(t *testing.T) {
	archive := tarArchive(t, "hello.txt", "share contents")
	base := Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte(`{}`), Database: []byte("SQLite format 3\x00")}
	for _, payload := range []SharePayload{
		{ID: "../escape", Name: "Documents", Path: "/srv/share", Archive: archive},
		{ID: "share-1", Name: "Documents", Path: "/etc/passwd", Archive: archive},
		{ID: "share-1", Name: "Documents", Path: "/srv/share", Archive: tarArchive(t, "../escape", "bad")},
	} {
		input := base
		input.Shares = []SharePayload{payload}
		if _, err := Create(input, []byte("key")); err == nil {
			t.Fatalf("invalid share payload was accepted: %#v", payload)
		}
	}
	base.Shares = []SharePayload{
		{ID: "share-parent", Name: "Parent", Path: "/srv/share", Archive: archive},
		{ID: "share-child", Name: "Child", Path: "/srv/share/child", Archive: archive},
	}
	if _, err := Create(base, []byte("key")); err == nil {
		t.Fatal("overlapping share roots were accepted")
	}
	base.Shares = []SharePayload{{ID: "share-1", Name: "Documents", Path: "/srv/share", Archive: archive}}
	bundle, err := Create(base, []byte("right-key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(bundle, []byte("wrong-key")); err == nil || !strings.Contains(err.Error(), "decryption") {
		t.Fatalf("wrong recovery key was accepted: %v", err)
	}
}

func tarArchive(t *testing.T, name, content string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gz)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o640, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
