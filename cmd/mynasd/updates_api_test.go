package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/updates"
)

func TestUpdateAPIRequiresVerifiedRecoveryAndSupportsABHealthRollback(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	updateRoot := filepath.Join(root, "updates")
	recoveryRoot := filepath.Join(root, "recovery")
	key := "test-recovery-key"
	t.Setenv("MYNAS_UPDATE_ROOT", updateRoot)
	t.Setenv("MYNAS_RECOVERY_DIR", recoveryRoot)
	t.Setenv("MYNAS_RECOVERY_KEY", key)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYNAS_UPDATE_PUBLIC_KEY", hex.EncodeToString(publicKey))
	packagePath := filepath.Join(root, "update.pkg")
	packageData := []byte("signed update package")
	if err := os.WriteFile(packagePath, packageData, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256Hex(packageData)
	manifest := updates.Manifest{FormatVersion: updates.ManifestFormatVersion, Version: "0.2.0", PackageSHA256: digest, PackageSize: int64(len(packageData)), PublishedAt: time.Now().UTC()}
	canonical, err := updates.CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{FormatVersion: recovery.FormatVersion, ConfigSchema: 1, MyNASVersion: "0.1.0", NASUUID: "nas-test", Generation: 1}, DesiredState: []byte(`{"generation":1}`), Database: []byte("database")}, []byte(key))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(recoveryRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(recoveryRoot, "latest.mrb")
	if err := os.WriteFile(backupPath, bundle, 0o600); err != nil {
		t.Fatal(err)
	}

	requestBody, _ := json.Marshal(updateApplyRequest{Manifest: manifest, Signature: signature, PackagePath: packagePath, BackupPath: backupPath})
	applyResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(applyResponse, httptest.NewRequest(http.MethodPost, "/api/v1/updates/apply", strings.NewReader(string(requestBody))))
	if applyResponse.Code != http.StatusAccepted {
		t.Fatalf("apply status %d: %s", applyResponse.Code, applyResponse.Body.String())
	}

	health := httptest.NewRecorder()
	server.routes().ServeHTTP(health, httptest.NewRequest(http.MethodPost, "/api/v1/updates/health", strings.NewReader(`{"healthy":true,"version":"0.2.0"}`)))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"activeSlot":"b"`) {
		t.Fatalf("health status %d: %s", health.Code, health.Body.String())
	}

	rollback := httptest.NewRecorder()
	server.routes().ServeHTTP(rollback, httptest.NewRequest(http.MethodPost, "/api/v1/updates/rollback", strings.NewReader(`{"reason":"smoke check failed"}`)))
	if rollback.Code != http.StatusOK || !strings.Contains(rollback.Body.String(), `"activeSlot":"a"`) {
		t.Fatalf("rollback status %d: %s", rollback.Code, rollback.Body.String())
	}
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
