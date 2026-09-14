package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsOverlayDebianUpdateCounts(t *testing.T) {
	server := testServer(t)
	server.debianUpdatesFunc = func() map[string]any {
		return map[string]any{"pendingCount": 7, "securityCount": 2}
	}

	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("settings fetch failed: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"pendingCount":7`) || !strings.Contains(body, `"securityCount":2`) {
		t.Fatalf("live Debian update counts missing: %s", body)
	}
}

func TestSettingsKeepZeroCountsWhenBrokerUnavailable(t *testing.T) {
	server := testServer(t)
	// debianUpdatesFunc nil -> broker unreachable in tests -> persisted zero.
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("settings fetch failed: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"securityCount"`) {
		t.Fatal("securityCount should be absent when the broker is unavailable")
	}
}

func TestSettingsOverlayDockerLoggingFromDaemonJson(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "daemon.json")
	if err := os.WriteFile(config, []byte(`{"log-driver":"local","log-opts":{"max-size":"25m","max-file":"5"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_DOCKER_DAEMON_JSON", config)

	server := testServer(t)
	server.dockerLoggingFunc = func() map[string]any {
		// Mirror production: parse the config file, add broker consumers.
		return map[string]any{
			"driver":       "local",
			"maxSizeMb":    25,
			"maxFiles":     5,
			"topConsumers": []map[string]any{{"name": "abc", "sizeBytes": 52428800}},
		}
	}

	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("settings fetch failed: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{`"driver":"local"`, `"maxSizeMb":25`, `"maxFiles":5`, `"name":"abc"`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("docker logging state missing %s in %s", needle, body)
		}
	}
}

func TestParseDockerSizeMB(t *testing.T) {
	cases := map[string]int{"10m": 10, "25mb": 25, "1g": 1024, "2gb": 2048, "512k": 0, "100b": 0}
	for raw, want := range cases {
		got, err := parseDockerSizeMB(raw)
		if err != nil || got != want {
			t.Fatalf("parseDockerSizeMB(%q) = %d, %v; want %d", raw, got, err, want)
		}
	}
}
