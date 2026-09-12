package shares

import (
	"strings"
	"testing"
)

func TestRenderFTPGlobalConfigWithoutTLSForPlainFTP(t *testing.T) {
	values := []ManagedShare{{
		Name: "Docs", Path: "/srv/docs", Enabled: true,
		Protocols: []Protocol{{Name: "ftp", Settings: map[string]any{"readOnly": true}}},
		Access:    []AccessRule{{PrincipalName: "guest", Level: "write"}},
	}}
	config, err := RenderFTPConfig(values, FTPOptions{UserConfigDir: "/generated/vsftpd-users"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(config.Main, "ssl_enable") {
		t.Fatalf("plain FTP must not enable TLS:\n%s", config.Main)
	}
	if !strings.Contains(config.Main, "pasv_min_port=50000") || !strings.Contains(config.Main, "pasv_max_port=51000") {
		t.Fatalf("expected default passive range:\n%s", config.Main)
	}
	if user := config.Users["guest"]; !strings.Contains(user, "local_root=/srv/docs") || !strings.Contains(user, "write_enable=NO") {
		t.Fatalf("read-only share must force write_enable=NO:\n%s", user)
	}
}

func TestRenderFTPRejectsMultiRootUsersAndUnsafeInput(t *testing.T) {
	shared := []ManagedShare{
		{Name: "One", Path: "/srv/one", Enabled: true, Protocols: []Protocol{{Name: "ftp"}}, Access: []AccessRule{{PrincipalName: "family", Level: "read"}}},
		{Name: "Two", Path: "/srv/two", Enabled: true, Protocols: []Protocol{{Name: "ftp"}}, Access: []AccessRule{{PrincipalName: "family", Level: "read"}}},
	}
	if _, err := RenderFTPConfig(shared, FTPOptions{UserConfigDir: "/generated"}); err == nil || !strings.Contains(err.Error(), "multiple FTP roots") {
		t.Fatalf("expected multi-root rejection, got %v", err)
	}
	if _, err := RenderFTPConfig(shared[:1], FTPOptions{}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected user dir requirement, got %v", err)
	}
	ftpsOnly := []ManagedShare{{Name: "Sec", Path: "/srv/sec", Enabled: true, Protocols: []Protocol{{Name: "ftps"}}}}
	if _, err := RenderFTPConfig(ftpsOnly, FTPOptions{UserConfigDir: "/generated"}); err == nil || !strings.Contains(err.Error(), "TLS certificate") {
		t.Fatalf("expected TLS path requirement, got %v", err)
	}
}

func TestRenderFTPSkipsDisabledSharesAndNoneRules(t *testing.T) {
	values := []ManagedShare{
		{Name: "Off", Path: "/srv/off", Enabled: false, Protocols: []Protocol{{Name: "ftp"}}, Access: []AccessRule{{PrincipalName: "nobody", Level: "write"}}},
		{Name: "On", Path: "/srv/on", Enabled: true, Protocols: []Protocol{{Name: "ftp"}}, Access: []AccessRule{{PrincipalName: "nobody", Level: "none"}, {PrincipalName: "family", Level: "write"}}},
	}
	config, err := RenderFTPConfig(values, FTPOptions{UserConfigDir: "/generated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Users) != 1 {
		t.Fatalf("expected only family mapped: %#v", config.Users)
	}
	if user := config.Users["family"]; !strings.Contains(user, "write_enable=YES") {
		t.Fatalf("write level should allow writes:\n%s", user)
	}
}
