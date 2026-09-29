package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

func TestTLSCertificateStatusReportsExpiryWithoutKeyMaterial(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "nas.example.test"}, DNSNames: []string{"nas.example.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(10 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "server.crt")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	encodedKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "server.key")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_WEB_TLS_CERT", path)
	t.Setenv("LUMONAS_WEB_TLS_KEY", keyPath)
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/security/certificate", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"expiring"`) || !strings.Contains(response.Body.String(), "nas.example.test") {
		t.Fatalf("certificate status did not include expected validity summary: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatal("certificate status exposed private-key material")
	}
}

func TestTLSCertificateStatusReportsIncompletePair(t *testing.T) {
	t.Setenv("LUMONAS_WEB_TLS_CERT", filepath.Join(t.TempDir(), "server.crt"))
	t.Setenv("LUMONAS_WEB_TLS_KEY", "")
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/security/certificate", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"incomplete"`) || !strings.Contains(response.Body.String(), `"configured":false`) {
		t.Fatalf("incomplete TLS configuration was misreported: %d %s", response.Code, response.Body.String())
	}
}

func TestTLSCertificateImportValidatesAndQueuesPrivilegedRotation(t *testing.T) {
	certificatePEM, keyPEM := makeTLSCertificatePair(t)
	generatedDir := t.TempDir()
	t.Setenv("LUMONAS_GENERATED_DIR", generatedDir)
	server := testServer(t)
	called := make(chan struct{}, 1)
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		if request.Operation != "tls.certificate.install" || !request.Confirmed || request.OperationID == "" {
			t.Errorf("unexpected privileged TLS request: %#v", request)
		}
		id, _ := request.RequestedState["certificateId"].(string)
		if id == "" {
			t.Errorf("certificate ID missing from privileged request")
		} else {
			if _, err := os.Stat(filepath.Join(generatedDir, id+".crt")); err != nil {
				t.Errorf("certificate staging file missing: %v", err)
			}
			if info, err := os.Stat(filepath.Join(generatedDir, id+".key")); err != nil {
				t.Errorf("private key staging file missing: %v", err)
			} else if info.Mode().Perm() != 0o600 {
				t.Errorf("staged private key has permissive mode %o", info.Mode().Perm())
			}
		}
		called <- struct{}{}
		return nil
	}
	body, err := json.Marshal(tlsCertificateImportRequest{Certificate: string(certificatePEM), PrivateKey: string(keyPEM)})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/security/certificate/import", bytes.NewReader(body)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("valid certificate import was not queued: %d %s", response.Code, response.Body.String())
	}
	var queued model.Job
	if err := json.Unmarshal(response.Body.Bytes(), &queued); err != nil || queued.State != "queued" {
		t.Fatalf("certificate install job response is invalid: %#v err=%v", queued, err)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("privileged certificate installation was not started")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := server.store.Job(queued.ID)
		if err == nil && job.State == "successful" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := server.store.Job(queued.ID)
	t.Fatalf("certificate install job did not finish successfully: %#v", job)
}

func TestTLSCertificateImportRejectsMismatchedKey(t *testing.T) {
	certificatePEM, _ := makeTLSCertificatePair(t)
	_, otherKey := makeTLSCertificatePair(t)
	server := testServer(t)
	body, err := json.Marshal(tlsCertificateImportRequest{Certificate: string(certificatePEM), PrivateKey: string(otherKey)})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/security/certificate/import", bytes.NewReader(body)))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "do not match") {
		t.Fatalf("mismatched certificate pair was accepted: %d %s", response.Code, response.Body.String())
	}
	jobs, err := server.store.Jobs()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("invalid certificate created a job: jobs=%v err=%v", jobs, err)
	}
}

func makeTLSCertificatePair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: big.NewInt(19), Subject: pkix.Name{CommonName: "nas.example.test"}, DNSNames: []string{"nas.example.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(20 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
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
