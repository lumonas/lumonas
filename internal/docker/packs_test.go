package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeImagePack(t *testing.T, root, name, manifest string, archives map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for file, payload := range archives {
		path := filepath.Join(dir, file)
		if err := os.WriteFile(path, []byte(payload), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func digestOf(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func validPackManifest(payload string) string {
	return `{"name":"media-pack","version":"1.0.0","images":[{"file":"app.tar","repository":"example/app","tag":"1.0","sha256":"` + digestOf(payload) + `","sizeBytes":` + strconv.Itoa(len(payload)) + `}]}`
}

func TestLoadImagePackValidatesManifestAndArchives(t *testing.T) {
	root := t.TempDir()
	payload := "docker save payload"
	dir := writeImagePack(t, root, "media-pack", validPackManifest(payload), map[string]string{"app.tar": payload})

	pack, err := LoadImagePack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Name != "media-pack" || len(pack.Images) != 1 || pack.Images[0].Repository != "example/app" {
		t.Fatalf("unexpected pack %#v", pack)
	}

	// A manifest whose declared size contradicts the archive is structurally
	// invalid; the archive itself is untouched.
	tampered := `{"name":"media-pack","images":[{"file":"app.tar","sha256":"` + digestOf(payload) + `","sizeBytes":99999}]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(tampered), 0o640); err != nil {
		t.Fatal(err)
	}
	service := New(t.TempDir(), func(context.Context, string, ...string) ([]byte, error) { return nil, nil })
	if _, err := service.ImportImagePack(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "declares") {
		t.Fatalf("expected size mismatch to abort the import, got %v", err)
	}
}

func TestLoadImagePackRejectsUnsafeEntries(t *testing.T) {
	root := t.TempDir()
	cases := map[string]string{
		"path traversal":  `{"name":"pack","images":[{"file":"../escape.tar","sha256":"` + strings.Repeat("a", 64) + `"}]}`,
		"missing digest":  `{"name":"pack","images":[{"file":"app.tar","sha256":"nothex"}]}`,
		"no images":       `{"name":"pack","images":[]}`,
		"absolute file":   `{"name":"pack","images":[{"file":"/etc/passwd","sha256":"` + strings.Repeat("a", 64) + `"}]}`,
		"invalid name":    `{"name":"Bad Name!","images":[{"file":"app.tar","sha256":"` + strings.Repeat("a", 64) + `"}]}`,
	}
	for label, manifest := range cases {
		dir := writeImagePack(t, root, "unsafe-pack", manifest, nil)
		if _, err := LoadImagePack(dir); err == nil {
			t.Fatalf("%s: expected rejection", label)
		}
	}
}

func TestImportImagePackVerifiesThenImportsEveryArchive(t *testing.T) {
	root := t.TempDir()
	first, second := "first image payload", "second image payload"
	manifest := `{"name":"media-pack","images":[` +
		`{"file":"app.tar","repository":"example/app","tag":"1.0","sha256":"` + digestOf(first) + `","sizeBytes":` + strconv.Itoa(len(first)) + `},` +
		`{"file":"db.tar","repository":"example/db","tag":"2.0","sha256":"` + digestOf(second) + `","sizeBytes":` + strconv.Itoa(len(second)) + `}]}`
	dir := writeImagePack(t, root, "media-pack", manifest, map[string]string{"app.tar": first, "db.tar": second})

	var commands []string
	service := New(t.TempDir(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	result, err := service.ImportImagePack(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 2 || len(result.Failed) != 0 {
		t.Fatalf("unexpected result %#v", result)
	}
	if strings.Join(result.Imported, ",") != "example/app:1.0,example/db:2.0" {
		t.Fatalf("unexpected imported refs %#v", result.Imported)
	}
	joined := strings.Join(commands, "\n")
	if strings.Count(joined, "docker load --input "+dir) != 2 {
		t.Fatalf("expected both archives loaded: %s", joined)
	}

	// A corrupted archive aborts the whole import before any docker call.
	// The replacement keeps the declared size so the digest branch is what
	// catches it.
	if err := os.WriteFile(filepath.Join(dir, "db.tar"), []byte("TAMPERED IMAGE BYTE"), 0o640); err != nil {
		t.Fatal(err)
	}
	commands = nil
	result, err = service.ImportImagePack(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
	if len(result.Imported) != 0 || len(commands) != 0 {
		t.Fatalf("import ran despite failed verification: %#v %v", result.Imported, commands)
	}
}

func TestAvailableImagePacksSkipsInvalidDirectories(t *testing.T) {
	root := t.TempDir()
	payload := "payload"
	writeImagePack(t, root, "good-pack", validPackManifest(payload), map[string]string{"app.tar": payload})
	writeImagePack(t, root, "broken-pack", `{"name":"broken","images":[{"file":"x.tar","sha256":"`+strings.Repeat("a", 64)+`"}]}`, nil)

	packs, err := AvailableImagePacks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].Name != "media-pack" || packs[0].ImageCount != 1 || packs[0].TotalSizeBytes != uint64(len(payload)) {
		t.Fatalf("unexpected packs %#v", packs)
	}
	// Missing root is an empty list, not an error.
	empty, err := AvailableImagePacks(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(empty) != 0 {
		t.Fatalf("unexpected result %#v err=%v", empty, err)
	}
}

func TestImportImageRejectsNonRegularPackFiles(t *testing.T) {
	root := t.TempDir()
	payload := "payload"
	dir := writeImagePack(t, root, "link-pack", validPackManifest(payload), map[string]string{"app.tar": payload})
	if err := os.Remove(filepath.Join(dir, "app.tar")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.tar"), []byte(payload), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.tar"), filepath.Join(dir, "app.tar")); err != nil {
		t.Skip("symlinks unavailable")
	}
	service := New(t.TempDir(), func(context.Context, string, ...string) ([]byte, error) { return nil, nil })
	if _, err := service.ImportImagePack(context.Background(), dir); err == nil {
		t.Fatal("expected symlinked archive to be rejected")
	}
}
