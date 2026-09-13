package collector

import (
	"context"
	"errors"
	"testing"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
)

func TestDockerSummaryUsesControlledRuntimeService(t *testing.T) {
	// The injected runner is intentionally CLI-shaped for this compatibility
	// test; production Service values use the Docker Engine API client.
	service := dockerruntime.New("", func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "docker" {
			return nil, errors.New("unexpected command")
		}
		if len(args) >= 2 && args[0] == "version" {
			return []byte("27.0"), nil
		}
		return []byte(`{"ID":"container-1","Names":"media","Image":"example/media:latest","State":"Up 2 hours","Ports":"","Labels":""}` + "\n"), nil
	})

	got := DockerSummaryFromService(context.Background(), service)
	if !got.Available || got.AppsRunning != 1 {
		t.Fatalf("unexpected Docker summary: %#v", got)
	}
}

func TestDockerSummaryReportsUnavailableRuntime(t *testing.T) {
	service := dockerruntime.New("", func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("Docker Engine unavailable")
	})
	got := DockerSummaryFromService(context.Background(), service)
	if got.Available || got.AppsRunning != 0 {
		t.Fatalf("unavailable Docker summary was not empty: %#v", got)
	}
}
