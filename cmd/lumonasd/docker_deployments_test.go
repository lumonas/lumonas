package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/model"
)

func TestStackUpdateDeploymentRestoresComposeAfterHealthGateFailure(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "media")
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	before := "services:\n  media:\n    image: example/media:1\n"
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte(before), 0o640); err != nil {
		t.Fatal(err)
	}
	server := testServer(t)
	server.log = slog.Default()
	server.deploymentOptions = deploymentOptions{GracePeriod: time.Millisecond, Interval: time.Millisecond, Timeout: 20 * time.Millisecond}
	server.dockerService = dockerruntime.New(root, func(_ context.Context, name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		if strings.HasSuffix(command, "images --format json") {
			return []byte(`{"Repository":"example/media","Tag":"2","ID":"sha256:old"}` + "\n"), nil
		}
		return nil, nil
	})
	_, err := server.runStackUpdateDeployment(context.Background(), dockerruntime.Stack{ID: "stack-media", Name: "media"}, "services:\n  media:\n    image: example/media:2\n", func(context.Context) error {
		return errors.New("container is restarting")
	})
	if err == nil {
		t.Fatal("expected health gate failure")
	}
	restored, err := os.ReadFile(filepath.Join(stackDir, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != before {
		t.Fatalf("failed update did not restore old compose: %q", restored)
	}
	history, err := server.store.DockerDeployments(10, "media")
	if err != nil || len(history) != 1 || history[0].State != "rolled_back" {
		t.Fatalf("unexpected deployment history: %#v err=%v", history, err)
	}
}

func TestDockerDeploymentHistoryRoute(t *testing.T) {
	server := testServer(t)
	if err := server.store.CreateDockerDeployment(model.DockerDeployment{ID: "deployment-route", StackName: "media", Kind: "install", ComposeAfter: "services:\n  media:\n    image: example/media\n"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/docker/deployments?limit=10", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	var deployments []model.DockerDeployment
	if err := json.NewDecoder(response.Body).Decode(&deployments); err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 1 || deployments[0].ID != "deployment-route" {
		t.Fatalf("unexpected deployment response: %#v", deployments)
	}
}
