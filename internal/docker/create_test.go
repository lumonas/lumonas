package docker

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCreateStackWritesComposeAtomically(t *testing.T) {
	root := t.TempDir()
	service := New(root, func(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrNotExist })
	stack, err := service.CreateStack("media", "services:\n  media:\n    image: jellyfin/jellyfin:latest\n")
	if err != nil {
		t.Fatal(err)
	}
	if stack.ID != "stack-media" {
		t.Fatal(stack.ID)
	}
	if _, err := service.CreateStack("media", "services:"); err == nil {
		t.Fatal("duplicate stack should fail")
	}
}

func TestLoadCatalog(t *testing.T) {
	path := t.TempDir() + "/apps.json"
	if err := os.WriteFile(path, []byte(`[{"id":"jellyfin","name":"Jellyfin"}]`), 0o640); err != nil {
		t.Fatal(err)
	}
	apps, err := LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].ID != "jellyfin" {
		t.Fatalf("unexpected catalog %#v", apps)
	}
}

func TestBuildComposeFromCatalogKeepsSecretsAsReferences(t *testing.T) {
	compose, err := BuildCompose(CatalogApp{ID: "media", Image: "example/media:1", Ports: []int{8096}, Form: []CatalogFormField{{ID: "PORT", Type: "port", DefaultValue: "8096"}, {ID: "PASSWORD", Type: "secret"}, {ID: "TZ", Type: "timezone"}}}, "media", map[string]string{"PORT": "18096", "PASSWORD": "do-not-write", "TZ": "UTC"}, []StorageMapping{{ContainerPath: "/media", ResourceID: "share-media"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compose, "18096:8096") || !strings.Contains(compose, "PASSWORD: \"${PASSWORD}\"") || strings.Contains(compose, "do-not-write") {
		t.Fatalf("unexpected compose: %s", compose)
	}
}
