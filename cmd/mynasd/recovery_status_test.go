package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/recovery"
)

func TestRecoveryStatusDoesNotMarkInvalidDatabaseAsVerified(t *testing.T) {
	server := testServer(t)
	directory := t.TempDir()
	t.Setenv("MYNAS_RECOVERY_KEY", "status-key")
	t.Setenv("MYNAS_RECOVERY_DIR", directory)
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{NASUUID: "nas-test"}, DesiredState: []byte("{}"), Database: []byte("not-sqlite")}, []byte("status-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "latest.mrb"), bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/recovery/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var status struct {
		Verified bool `json:"verified"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Verified {
		t.Fatal("invalid database payload must not be reported as verified")
	}
}
