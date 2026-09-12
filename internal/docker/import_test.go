package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportImageUsesTypedDockerLoad(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "image.tar")
	if err := os.WriteFile(archive, []byte("docker archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	var command string
	service := New(t.TempDir(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	if err := service.ImportImage(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	if command != "docker load --input "+archive {
		t.Fatalf("unexpected import command: %q", command)
	}
}

func TestImportImageRejectsRelativeOrMissingArchive(t *testing.T) {
	service := New(t.TempDir(), nil)
	if err := service.ImportImage(context.Background(), "relative.tar"); err == nil {
		t.Fatal("relative archive path should fail")
	}
	if err := service.ImportImage(context.Background(), filepath.Join(t.TempDir(), "missing.tar")); err == nil {
		t.Fatal("missing archive should fail")
	}
}
