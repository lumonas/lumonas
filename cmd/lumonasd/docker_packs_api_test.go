package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

func writeTestImagePack(t *testing.T, root string) string {
	t.Helper()
	directory := filepath.Join(root, "media-pack")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatal(err)
	}
	payload := []byte("payload")
	digest := sha256.Sum256(payload)
	manifest := fmt.Sprintf(`{"name":"media-pack","version":"1.0.0","images":[{"file":"app.tar","repository":"example/app","tag":"1.0","sha256":"%s","sizeBytes":%d}]}`, hex.EncodeToString(digest[:]), len(payload))
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), []byte(manifest), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "app.tar"), payload, 0o640); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestDockerImagePacksRouteListsValidatedPacks(t *testing.T) {
	root := t.TempDir()
	writeTestImagePack(t, root)
	server := testServer(t)
	t.Setenv("LUMONAS_IMAGE_PACK_ROOT", root)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/docker/images/packs", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var packs []dockerruntime.ImagePackSummary
	if err := json.NewDecoder(response.Body).Decode(&packs); err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].Name != "media-pack" {
		t.Fatalf("unexpected pack list: %#v", packs)
	}
}

func TestDockerImagePackImportIsRootScoped(t *testing.T) {
	root := t.TempDir()
	packPath := writeTestImagePack(t, root)
	server := testServer(t)
	server.dockerService = dockerruntime.New(t.TempDir(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "docker" || !strings.Contains(strings.Join(args, " "), "load --input "+packPath) {
			t.Fatalf("unexpected Docker command: %s %s", name, strings.Join(args, " "))
		}
		return nil, nil
	})
	t.Setenv("LUMONAS_IMAGE_PACK_ROOT", root)
	response := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"media-pack"}`)
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/docker/images/packs/import", body))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"pack":"media-pack"`) {
		t.Fatalf("unexpected import response %d: %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	body = strings.NewReader(`{"name":"../outside-pack"}`)
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/docker/images/packs/import", body))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("outside pack path was accepted: %d %s", response.Code, response.Body.String())
	}
}
