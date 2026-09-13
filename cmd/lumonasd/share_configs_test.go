package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestFTPOptionsMatchProvisionedWebTLSCertificate(t *testing.T) {
	t.Setenv("LUMONAS_WEB_TLS_CERT", "")
	t.Setenv("LUMONAS_WEB_TLS_KEY", "")
	options := ftpOptions()
	if options.TLSCertFile != "/etc/lumonas/tls/server.crt" || options.TLSKeyFile != "/etc/lumonas/tls/server.key" {
		t.Fatalf("FTPS defaults do not match Debian certificate paths: %#v", options)
	}
}

func TestPrepareShareConfigsUsesProvisionedFTPSCertificate(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LUMONAS_FTP_CONFIG", filepath.Join(directory, "ftp.conf"))
	t.Setenv("LUMONAS_FTP_USER_DIR", filepath.Join(directory, "users"))
	t.Setenv("LUMONAS_WEB_TLS_CERT", "")
	t.Setenv("LUMONAS_WEB_TLS_KEY", "")

	prepared, err := (&apiServer{}).prepareShareConfigs([]shares.ManagedShare{{
		Name: "Media", Path: "/srv/media", Enabled: true,
		Protocols: []shares.Protocol{{Name: "ftps"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := activateShareConfigs(prepared); err != nil {
		t.Fatal(err)
	}
	defer restoreShareConfigs(prepared)
	content, err := os.ReadFile(filepath.Join(directory, "ftp.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "rsa_cert_file=/etc/lumonas/tls/server.crt") || !strings.Contains(string(content), "rsa_private_key_file=/etc/lumonas/tls/server.key") {
		t.Fatalf("generated FTPS config uses unexpected certificate paths:\n%s", content)
	}
}

func TestReloadShareServicesFailsClosedWithoutPrivilegedBroker(t *testing.T) {
	server := &apiServer{}
	t.Setenv("LUMONAS_PRIVD_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	err := server.reloadShareServices(context.Background(), []shares.ManagedShare{{
		Name: "Media", Path: "/srv/media", Enabled: true,
		Protocols: []shares.Protocol{{Name: "smb"}},
	}})
	if err == nil {
		t.Fatal("share activation unexpectedly succeeded without the privileged broker")
	}
}

func TestReloadShareServicesUsesTypedBrokerForEveryActivation(t *testing.T) {
	server := &apiServer{}
	requests := make([]privileged.Request, 0)
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		requests = append(requests, request)
		return nil
	}
	err := server.reloadShareServices(context.Background(), []shares.ManagedShare{{
		Name: "Media", Path: "/srv/media", Enabled: true,
		Protocols: []shares.Protocol{{Name: "smb"}, {Name: "nfs"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("expected Avahi plus two service activations, got %d: %#v", len(requests), requests)
	}
	for _, request := range requests {
		if request.OperationID == "" || request.PlanHash == "" || request.ExpiresAt.IsZero() || !request.Confirmed {
			t.Fatalf("activation request was not typed and confirmed: %#v", request)
		}
	}
}
