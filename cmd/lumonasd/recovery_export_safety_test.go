package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestRecoveryExportFailsClosedWhenDiskIdentityCollectionFails(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return nil, errors.New("collector unavailable")
	}
	t.Setenv("LUMONAS_RECOVERY_KEY", "recovery-export-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", t.TempDir())

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/recovery/export", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "disk identity export failed") {
		t.Fatalf("expected disk collection failure, got %d: %s", response.Code, response.Body.String())
	}
}

func TestRecoveryExportRequiresNASIdentity(t *testing.T) {
	server := testServer(t)
	if err := server.store.SetMeta("nas_uuid", ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_RECOVERY_KEY", "recovery-export-key")
	t.Setenv("LUMONAS_RECOVERY_DIR", t.TempDir())

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/recovery/export", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "NAS identity is not configured") {
		t.Fatalf("expected NAS identity failure, got %d: %s", response.Code, response.Body.String())
	}
}
