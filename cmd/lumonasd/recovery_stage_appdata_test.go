package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/recovery"
)

func TestRecoveryStageAppdataStagesOnlyRequestedWorkload(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	recoveryDir := filepath.Join(root, "recovery")
	stagingDir := filepath.Join(root, "staged")
	t.Setenv("LUMONAS_RECOVERY_KEY", "stage-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", recoveryDir)
	t.Setenv("LUMONAS_RECOVERY_STAGING_DIR", stagingDir)
	source := filepath.Join(root, "appdata")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.json"), []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	archive, err := recovery.ArchiveAppdata(source, recovery.DefaultAppdataArchiveLimit)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := recovery.Create(recovery.Input{
		Manifest: recovery.Manifest{NASUUID: "nas-test", Generation: 9}, DesiredState: []byte("{}"), Database: []byte("SQLite format 3\x00staged"),
		Compose: map[string][]byte{"media/compose.yaml": []byte("services:\n  media:\n    image: example/media\n"), "other/compose.yaml": []byte("services:\n  other:\n    image: example/other\n")},
		Appdata: []recovery.AppdataPayload{
			{Stack: "media", ContainerPath: "/config", HostPath: "/srv/lumonas/docker/appdata/media", Archive: archive},
			{Stack: "other", ContainerPath: "/config", HostPath: "/srv/lumonas/docker/appdata/other", Archive: archive},
		},
	}, []byte("stage-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(recoveryDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recoveryDir, "latest.mrb"), bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/recovery/restore/stage-appdata", strings.NewReader(`{"stack":"media","confirmed":true,"reauthenticated":true}`)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Stack     string   `json:"stack"`
		Directory string   `json:"directory"`
		Files     []string `json:"files"`
		Verified  bool     `json:"verified"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Stack != "media" || !result.Verified || len(result.Files) != 2 || !containsRecoveryFile(result.Files, "docker/stacks/media/compose.yaml") {
		t.Fatalf("unexpected selective stage: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Directory, "docker", "stacks", "other", "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("unselected workload was staged: %v", err)
	}
}
