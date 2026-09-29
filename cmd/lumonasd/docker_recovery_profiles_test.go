package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

func TestDockerRecoveryProfileAddsValidatedPathsToStack(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "custom-media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  media:\n    image: example/media:latest\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := testServer(t)
	server.catalogFile = filepath.Join(t.TempDir(), "missing-catalog.json")
	server.dockerService = dockerruntime.New(root, func(context.Context, string, ...string) ([]byte, error) { return nil, nil })
	request := httptest.NewRequest(http.MethodPut, "/api/v1/docker/stacks/custom-media/recovery", strings.NewReader(`{"appdataPaths":["/config","/data","/config"]}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"recoveryCoverage":0.75`) {
		t.Fatalf("unexpected profile update: %d %s", response.Code, response.Body.String())
	}
	for _, invalid := range []string{"relative", "/config/../secret", "/"} {
		request = httptest.NewRequest(http.MethodPut, "/api/v1/docker/stacks/custom-media/recovery", strings.NewReader(`{"appdataPaths":["`+invalid+`"]}`))
		response = httptest.NewRecorder()
		server.routes().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid recovery path %q accepted: %d %s", invalid, response.Code, response.Body.String())
		}
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/docker/stacks", nil)
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"appdataPaths":["/config","/data"]`) {
		t.Fatalf("saved recovery profile was not returned on stack read: %d %s", response.Code, response.Body.String())
	}
}
