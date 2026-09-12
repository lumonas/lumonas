package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHKeysListReturnsEmptyWhenNoAuthorizedKeys(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SSH_AUTHORIZED_KEYS", filepath.Join(t.TempDir(), "authorized_keys"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ssh/keys", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var keys []sshKey
	if err := json.NewDecoder(rec.Body).Decode(&keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected empty list, got %d keys", len(keys))
	}
}

func TestSSHKeysAddAndList(t *testing.T) {
	server := testServer(t)
	keyFile := filepath.Join(t.TempDir(), "authorized_keys")
	t.Setenv("LUMONAS_SSH_AUTHORIZED_KEYS", keyFile)

	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/ssh/keys",
		strings.NewReader(`{"publicKey":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest test@host"}`))
	addRec := httptest.NewRecorder()
	server.routes().ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusOK {
		t.Fatalf("add failed: %d %s", addRec.Code, addRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/ssh/keys", nil)
	listRec := httptest.NewRecorder()
	server.routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list failed: %d", listRec.Code)
	}
	var keys []sshKey
	if err := json.NewDecoder(listRec.Body).Decode(&keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Comment != "test@host" {
		t.Fatalf("expected comment test@host, got %q", keys[0].Comment)
	}

	content, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "ssh-ed25519") {
		t.Fatal("authorized_keys file does not contain the added key")
	}
}

func TestSSHKeysRejectsInvalidKey(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SSH_AUTHORIZED_KEYS", filepath.Join(t.TempDir(), "authorized_keys"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh/keys",
		strings.NewReader(`{"publicKey":"not-a-valid-key"}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSSHKeysRemove(t *testing.T) {
	server := testServer(t)
	keyFile := filepath.Join(t.TempDir(), "authorized_keys")
	t.Setenv("LUMONAS_SSH_AUTHORIZED_KEYS", keyFile)

	if err := os.WriteFile(keyFile, []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest test@host\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh/keys/remove",
		strings.NewReader(`{"publicKey":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest"}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove failed: %d %s", rec.Code, rec.Body.String())
	}

	content, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "ssh-ed25519") {
		t.Fatal("key was not removed from authorized_keys")
	}
}

func TestSSHKeysAddEmptyKeyReturns400(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_SSH_AUTHORIZED_KEYS", filepath.Join(t.TempDir(), "authorized_keys"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ssh/keys",
		strings.NewReader(`{"publicKey":"  "}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty key, got %d", rec.Code)
	}
}
