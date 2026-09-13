package main

import (
	"context"
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

	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/updates"
)

func TestParseSlotDeviceMappingRejectsUnsafeAndDuplicateEntries(t *testing.T) {
	valid, err := parseSlotDeviceMapping("a=/dev/disk/by-partlabel/lumonas-a:0001,b=/dev/disk/by-partlabel/lumonas-b:0002")
	if err != nil || valid["a"].Device != "/dev/disk/by-partlabel/lumonas-a" {
		t.Fatalf("valid slot mapping rejected: %#v %v", valid, err)
	}
	for _, value := range []string{
		"a=/tmp/lumonas-a:0001,b=/dev/lumonas-b:0002",
		"a=/dev/lumonas-a:0001,a=/dev/lumonas-b:0002",
		"a=/dev/lumonas-a/../other:0001,b=/dev/lumonas-b:0002",
		"a=/dev/lumonas-a:xyz,b=/dev/lumonas-b:0002",
	} {
		if _, err := parseSlotDeviceMapping(value); err == nil {
			t.Fatalf("unsafe slot mapping accepted: %q", value)
		}
	}
}

func TestSlotAPIStagesActivatesAndConfirmsSignedImage(t *testing.T) {
	root := t.TempDir()
	updateRoot := filepath.Join(root, "updates")
	imagePath := filepath.Join(root, "rootfs.img")
	payload := []byte("signed root filesystem image")
	if err := os.WriteFile(imagePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	manifest := updates.Manifest{
		FormatVersion: updates.ManifestFormatVersion,
		Version:       "0.3.0",
		PackageSHA256: hex.EncodeToString(digest[:]),
		PackageSize:   int64(len(payload)),
		PublishedAt:   time.Now().UTC(),
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := updates.CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	setupsig := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	t.Setenv("LUMONAS_UPDATE_ROOT", updateRoot)
	t.Setenv("LUMONAS_UPDATE_PUBLIC_KEY", hex.EncodeToString(publicKey))
	t.Setenv("LUMONAS_SLOT_DEVICES", "a=/dev/disk/by-partlabel/lumonas-a:0001,b=/dev/disk/by-partlabel/lumonas-b:0002")
	recoveryRoot := filepath.Join(root, "recovery")
	recoveryKey := "slot-recovery-key"
	t.Setenv("LUMONAS_RECOVERY_DIR", recoveryRoot)
	t.Setenv("LUMONAS_RECOVERY_KEY", recoveryKey)
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{FormatVersion: recovery.FormatVersion, ConfigSchema: 1, LumoNASVersion: "0.2.0", NASUUID: "nas-test", Generation: 1}, DesiredState: []byte(`{"generation":1}`), Database: []byte("database")}, []byte(recoveryKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(recoveryRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recoveryRoot, "latest.mrb"), bundle, 0o600); err != nil {
		t.Fatal(err)
	}

	server := testServer(t)
	server.version = manifest.Version
	var requests []privileged.Request
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		requests = append(requests, request)
		return privileged.Response{OK: true}, nil
	}

	stageBody, err := json.Marshal(map[string]any{"imagePath": imagePath, "manifest": manifest, "signature": setupsig})
	if err != nil {
		t.Fatal(err)
	}
	stage := httptest.NewRecorder()
	server.routes().ServeHTTP(stage, httptest.NewRequest(http.MethodPost, "/api/v1/updates/slot/stage", strings.NewReader(string(stageBody))))
	if stage.Code != http.StatusAccepted || !strings.Contains(stage.Body.String(), `"pendingSlot":"b"`) {
		t.Fatalf("stage status %d: %s", stage.Code, stage.Body.String())
	}

	activate := httptest.NewRecorder()
	server.routes().ServeHTTP(activate, httptest.NewRequest(http.MethodPost, "/api/v1/updates/slot/activate", nil))
	if activate.Code != http.StatusAccepted || !strings.Contains(activate.Body.String(), `"bootNext":"0002"`) {
		t.Fatalf("activate status %d: %s", activate.Code, activate.Body.String())
	}
	if len(requests) != 2 || requests[0].Operation != "system.slot.write" || requests[1].Operation != "system.slot.bootnext" {
		t.Fatalf("unexpected privileged activation requests: %#v", requests)
	}
	if requests[0].OperationID == "" || requests[0].OperationID != requests[1].OperationID || requests[0].PlanHash != manifest.PackageSHA256 || requests[1].PlanHash != manifest.PackageSHA256 {
		t.Fatalf("slot activation lost operation correlation: %#v", requests)
	}

	confirm := httptest.NewRecorder()
	server.routes().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/updates/slot/confirm", nil))
	if confirm.Code != http.StatusOK || !strings.Contains(confirm.Body.String(), `"activeSlot":"b"`) || !strings.Contains(confirm.Body.String(), `"activeVersion":"0.3.0"`) {
		t.Fatalf("confirm status %d: %s", confirm.Code, confirm.Body.String())
	}
}
