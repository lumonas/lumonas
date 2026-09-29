package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/recovery"
)

func TestRecoveryBundleDownloadRequiresVerifiedBundle(t *testing.T) {
	server := testServer(t)
	directory := t.TempDir()
	t.Setenv("LUMONAS_RECOVERY_KEY", "download-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", directory)
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{NASUUID: "nas-test", Generation: 17}, DesiredState: []byte("{}"), Database: []byte("SQLite format 3\x00valid")}, []byte("download-key"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "latest.mrb")
	if err := os.WriteFile(path, bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/recovery/download", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Disposition"), "generation-17.mrb") || response.Body.String() != string(bundle) {
		t.Fatalf("unexpected download response: %d headers=%v bytes=%d", response.Code, response.Header(), response.Body.Len())
	}
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/recovery/download", nil))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("damaged recovery bundle was downloaded: %d %s", response.Code, response.Body.String())
	}
}
