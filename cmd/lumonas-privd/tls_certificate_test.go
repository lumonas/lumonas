package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tlsTestPair(t *testing.T, serial int64) ([]byte, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "nas.example.test"}, DNSNames: []string{"nas.example.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func TestInstallTLSCertificateFilesRestartsWebWithNewPair(t *testing.T) {
	certificate, key := tlsTestPair(t, 21)
	directory := t.TempDir()
	called := false
	err := installTLSCertificateFiles(certificate, key, directory, -1, func(name string, args ...string) ([]byte, error) {
		called = true
		if name != "systemctl" || strings.Join(args, " ") != "reload-or-restart lumonas-web.service" {
			t.Fatalf("unexpected service command: %s %v", name, args)
		}
		return nil, nil
	})
	if err != nil || !called {
		t.Fatalf("certificate installation failed: called=%v err=%v", called, err)
	}
	gotCertificate, err := os.ReadFile(filepath.Join(directory, "server.crt"))
	if err != nil || string(gotCertificate) != string(certificate) {
		t.Fatalf("installed certificate differs: err=%v", err)
	}
	gotKey, err := os.ReadFile(filepath.Join(directory, "server.key"))
	if err != nil || string(gotKey) != string(key) {
		t.Fatalf("installed key differs: err=%v", err)
	}
	info, err := os.Stat(filepath.Join(directory, "server.key"))
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("private key permissions are not 0640: info=%v err=%v", info, err)
	}
}

func TestInstallTLSCertificateFilesRestoresPairWhenWebRestartFails(t *testing.T) {
	certificate, key := tlsTestPair(t, 22)
	directory := t.TempDir()
	previousCertificate, previousKey := []byte("previous certificate"), []byte("previous key")
	if err := os.WriteFile(filepath.Join(directory, "server.crt"), previousCertificate, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "server.key"), previousKey, 0o640); err != nil {
		t.Fatal(err)
	}
	commands := 0
	err := installTLSCertificateFiles(certificate, key, directory, -1, func(string, ...string) ([]byte, error) {
		commands++
		if commands == 1 {
			return nil, os.ErrInvalid
		}
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "previous certificate was restored") || commands != 2 {
		t.Fatalf("restart failure was not reported with rollback: err=%v commands=%d", err, commands)
	}
	gotCertificate, readErr := os.ReadFile(filepath.Join(directory, "server.crt"))
	if readErr != nil || string(gotCertificate) != string(previousCertificate) {
		t.Fatalf("previous certificate was not restored: err=%v", readErr)
	}
	gotKey, readErr := os.ReadFile(filepath.Join(directory, "server.key"))
	if readErr != nil || string(gotKey) != string(previousKey) {
		t.Fatalf("previous key was not restored: err=%v", readErr)
	}
}

func TestTLSCertificateInstallRequiresConfirmationAndConstrainedID(t *testing.T) {
	setEnv := t.Setenv
	setEnv("LUMONAS_GENERATED_DIR", t.TempDir())
	for _, req := range []request{
		{Operation: "tls.certificate.install", OperationID: "tls-op-1", PlanHash: "tls-plan", Confirmed: false, RequestedState: map[string]any{"certificateId": "tls-0123456789abcdef"}},
		{Operation: "tls.certificate.install", OperationID: "tls-op-1", PlanHash: "tls-plan", Confirmed: true, RequestedState: map[string]any{"certificateId": "../../server"}},
	} {
		result := applyTLSCertificate(req, func(string, ...string) ([]byte, error) {
			t.Fatal("invalid TLS operation reached the service command")
			return nil, nil
		})
		if result.OK {
			t.Fatalf("unsafe TLS install request succeeded: %#v", req)
		}
	}
}

func TestTLSACMEConfigureValidatesConsentDomainAndEmailBeforeCommands(t *testing.T) {
	requests := []map[string]any{
		{"domain": "nas.example.com", "email": "admin@example.com", "agreeToTerms": false},
		{"domain": "*.example.com", "email": "admin@example.com", "agreeToTerms": true},
		{"domain": "nas.example.com", "email": "not-an-email", "agreeToTerms": true},
	}
	for index, values := range requests {
		called := false
		result := configureTLSACME(request{Operation: "tls.acme.configure", OperationID: "acme-op-1", PlanHash: "acme-plan", Confirmed: true, RequestedState: values}, func(string, ...string) ([]byte, error) {
			called = true
			t.Fatal("invalid ACME request reached certbot")
			return nil, nil
		})
		if result.OK || called || result.Error == "" {
			t.Fatalf("invalid ACME request %d was accepted: %#v", index, result)
		}
	}
	if operationWorker("tls.acme.configure") != "acme" || operationWorker("tls.acme.disable") != "acme" || !requiresOperationID("tls.acme.configure") || !requiresOperationID("tls.acme.disable") {
		t.Fatal("ACME operations are not confined to the dedicated worker and operation-ID gate")
	}
}

func TestDisableTLSACMERemovesRenewalProfileAndDeployHook(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LUMONAS_CERTBOT_DIR", root)
	renewalDir := filepath.Join(root, "renewal")
	hookDir := filepath.Join(root, "renewal-hooks", "deploy")
	if err := os.MkdirAll(renewalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(renewalDir, "lumonas-web.conf")
	hook := filepath.Join(hookDir, "lumonas-web")
	if err := os.WriteFile(profile, []byte("managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("hook"), 0o700); err != nil {
		t.Fatal(err)
	}
	called := false
	result := disableTLSACME(request{Operation: "tls.acme.disable", OperationID: "acme-disable-1", PlanHash: "acme-disable", Confirmed: true}, func(name string, args ...string) ([]byte, error) {
		called = true
		if name != "certbot" || strings.Join(args, " ") != "delete --non-interactive --cert-name lumonas-web" {
			t.Fatalf("unexpected Certbot deletion command: %s %v", name, args)
		}
		return nil, os.Remove(profile)
	})
	if !result.OK || !called {
		t.Fatalf("ACME renewal profile could not be disabled: %#v", result)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("renewal profile remains after disable: %v", err)
	}
	if _, err := os.Stat(hook); !os.IsNotExist(err) {
		t.Fatalf("renewal deploy hook remains after disable: %v", err)
	}
}
