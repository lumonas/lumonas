package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

func TestUnlockEncryptedDiskRequiresSafetyWindow(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{{ID: "wwn:test", CurrentPath: "/dev/sda", Filesystem: "crypto_LUKS"}}, nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/disks/wwn:test/unlock", strings.NewReader(`{"passphrase":"correct horse battery staple"}`)))
	if response.Code != http.StatusLocked {
		t.Fatalf("expected storage safety lock, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUnlockEncryptedDiskPassesSecretOnlyToPrivilegedWorker(t *testing.T) {
	server := testServer(t)
	server.safetyUntil = time.Now().UTC().Add(time.Minute)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{{ID: "wwn:test", CurrentPath: "/dev/sda", Filesystem: "crypto_LUKS", WWN: "wwn-value", SizeBytes: 42}}, nil
	}
	var captured privileged.Request
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		captured = request
		return privileged.Response{OK: true}, nil
	}
	const passphrase = "correct horse battery staple"
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/storage/disks/wwn:test/unlock", strings.NewReader(`{"passphrase":"`+passphrase+`"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("unlock failed: %d %s", response.Code, response.Body.String())
	}
	if captured.Operation != "filesystem.crypto.unlock" || captured.TargetDiskID != "wwn:test" || captured.RequestedState["mountPath"] != "/srv/disks/wwn_test" || captured.RequestedState["encryptionPassphrase"] != passphrase {
		t.Fatalf("unexpected privileged unlock request: %#v", captured)
	}
	if strings.Contains(response.Body.String(), passphrase) {
		t.Fatal("unlock response exposed the passphrase")
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
}
