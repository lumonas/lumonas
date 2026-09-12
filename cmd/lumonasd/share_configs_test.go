package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
