package docker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeRehearsalComposeRemovesExposureAndRewritesVolumes(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"services":{"web":{"image":"nginx:stable","ports":["8080:80"],"container_name":"live-web","volumes":[{"type":"bind","source":"/srv/data","target":"/data"},{"type":"volume","source":"web-cache","target":"/cache"}]}},"volumes":{"web-cache":{}},"networks":{"default":{}}}`)
	encoded, services, err := sanitizeRehearsalCompose(raw, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0] != "web" {
		t.Fatalf("services = %#v", services)
	}
	var config map[string]any
	if err := json.Unmarshal(encoded, &config); err != nil {
		t.Fatal(err)
	}
	service := config["services"].(map[string]any)["web"].(map[string]any)
	if _, exists := service["ports"]; exists {
		t.Fatal("published host ports were retained")
	}
	if _, exists := service["container_name"]; exists {
		t.Fatal("fixed container name was retained")
	}
	volumes := service["volumes"].([]any)
	bind := volumes[0].(map[string]any)
	if bind["source"] != filepath.Join(root, "srv/data") {
		t.Fatalf("bind mount source = %v", bind["source"])
	}
	if volumes[1].(map[string]any)["source"] != "drill-web-cache" {
		t.Fatalf("named volume was not isolated: %#v", volumes[1])
	}
	network := config["networks"].(map[string]any)["default"].(map[string]any)
	if network["internal"] != true {
		t.Fatalf("network is not isolated: %#v", network)
	}
}

func TestRehearseStackUsesIsolatedProjectAndAlwaysCleansUp(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "srv/lumonas/docker/stacks/web/compose.yaml")
	if err := os.MkdirAll(filepath.Dir(composePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composePath, []byte("services: {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	service := New("", func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		switch {
		case strings.Contains(strings.Join(args, " "), "config --format json"):
			return []byte(`{"services":{"web":{"image":"nginx:stable","ports":["8080:80"]}}}`), nil
		case strings.Contains(strings.Join(args, " "), " ps --format json"):
			return []byte(`{"Service":"web","State":"running","Health":"healthy"}`), nil
		default:
			return nil, nil
		}
	})
	result, err := service.RehearseStack(context.Background(), root, "web", time.Minute)
	if err != nil || !result.Healthy || result.NetworkMode != "private-internal" {
		t.Fatalf("rehearsal = %#v err=%v", result, err)
	}
	if len(calls) != 4 || !strings.Contains(calls[1], "up --detach --no-build --pull never") || !strings.Contains(calls[3], "down --volumes --remove-orphans") {
		t.Fatalf("unexpected Docker command lifecycle: %#v", calls)
	}
}

func TestSanitizeRehearsalRejectsPrivilegedAndExternalServices(t *testing.T) {
	for _, raw := range []string{
		`{"services":{"bad":{"image":"x","privileged":true}}}`,
		`{"services":{"web":{"image":"x"}},"networks":{"default":{"external":true}}}`,
		`{"services":{"web":{"image":"x"}},"volumes":{"outside":{"external":true}}}`,
	} {
		if _, _, err := sanitizeRehearsalCompose([]byte(raw), t.TempDir()); err == nil {
			t.Fatalf("unsafe Compose project was accepted: %s", raw)
		}
	}
}
