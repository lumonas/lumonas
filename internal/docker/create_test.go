package docker

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestVerifyCatalogSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`[{"id":"jellyfin"}]`)
	signature := ed25519.Sign(privateKey, data)
	encodedKey := base64.StdEncoding.EncodeToString(publicKey)
	encodedSignature := base64.StdEncoding.EncodeToString(signature)
	if !VerifyCatalogSignature(data, encodedSignature, encodedKey) {
		t.Fatal("valid catalog signature was rejected")
	}
	if VerifyCatalogSignature(append(data, ' '), encodedSignature, encodedKey) {
		t.Fatal("signature for different catalog bytes was accepted")
	}
	if VerifyCatalogSignature(data, encodedSignature, "invalid-key") {
		t.Fatal("malformed trust key was accepted")
	}
}

func TestCatalogInstallTrustPolicy(t *testing.T) {
	for _, tc := range []struct {
		status      string
		require     bool
		wantAllowed bool
	}{
		{CatalogTrustVerified, false, true},
		{CatalogTrustVerified, true, true},
		{CatalogTrustUnverified, false, true},
		{CatalogTrustUnverified, true, false},
		{CatalogTrustInvalid, false, false},
		{CatalogTrustInvalid, true, false},
	} {
		if got := CatalogInstallAllowed(CatalogApp{TrustStatus: tc.status}, tc.require); got != tc.wantAllowed {
			t.Errorf("CatalogInstallAllowed(%q, %t) = %t, want %t", tc.status, tc.require, got, tc.wantAllowed)
		}
	}
}

func TestLoadCatalogReportsSignatureTrust(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/apps.json"
	data := []byte(`[{"id":"jellyfin","name":"Jellyfin"}]`)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_CATALOG_PUBLIC_KEY", base64.StdEncoding.EncodeToString(publicKey))
	if _, err := LoadCatalog(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".sig", []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, data))), 0o640); err != nil {
		t.Fatal(err)
	}
	apps, err := LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].TrustStatus != CatalogTrustVerified {
		t.Fatalf("signed catalog trust not surfaced: %#v", apps)
	}
	if err := os.WriteFile(path+".sig", []byte(base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))), 0o640); err != nil {
		t.Fatal(err)
	}
	apps, err = LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if apps[0].TrustStatus != CatalogTrustInvalid {
		t.Fatalf("invalid signature trust not surfaced: %#v", apps[0])
	}
}

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

func TestValidateComposeRejectsEmptyServices(t *testing.T) {
	service := New(t.TempDir(), func(context.Context, string, ...string) ([]byte, error) { return nil, os.ErrNotExist })
	if err := service.ValidateCompose(context.Background(), "services:\n"); err == nil {
		t.Fatal("expected empty services mapping to be rejected")
	}
	if err := service.ValidateCompose(context.Background(), "services:\n  media:\n    image: example/media:latest\n"); err != nil {
		t.Fatalf("expected valid compose, got %v", err)
	}
}

func TestContainerAndImageActionsUseTypedDockerCommands(t *testing.T) {
	commands := make([]string, 0, 2)
	service := New(t.TempDir(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if err := service.ContainerAction(context.Background(), "media", "restart"); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateImage(context.Background(), Image{Repo: "jellyfin/jellyfin", Tag: "latest"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || commands[0] != "docker restart media" || commands[1] != "docker pull jellyfin/jellyfin:latest" {
		t.Fatalf("unexpected commands: %#v", commands)
	}
}
