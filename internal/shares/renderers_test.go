package shares

import (
	"os"
	"strings"
	"testing"
)

func TestProtocolRenderersAndValidatedAtomicWrite(t *testing.T) {
	values := []ManagedShare{{
		Name: "Media", Path: "/srv/media", Enabled: true,
		Protocols: []Protocol{
			{Name: "nfs", Settings: map[string]any{"allowedNetworks": []any{"192.168.1.0/24"}}},
			{Name: "sftp"}, {Name: "ftps", Settings: map[string]any{"passivePortStart": 40000, "passivePortEnd": 40100}},
			{Name: "rsync", Settings: map[string]any{"readOnly": true}},
		},
		Access: []AccessRule{{PrincipalName: "family", Level: "read"}},
	}}
	nfs, err := RenderNFS(values)
	if err != nil || !strings.Contains(nfs, "root_squash") || !strings.Contains(nfs, "192.168.1.0/24") {
		t.Fatalf("unexpected NFS config %q err=%v", nfs, err)
	}
	sftp, err := RenderSFTP(values)
	if err != nil || !strings.Contains(sftp, "ChrootDirectory /srv/media") {
		t.Fatalf("unexpected SFTP config %q err=%v", sftp, err)
	}
	ftp, err := RenderFTP(values)
	if err != nil || !strings.Contains(ftp, "passive_start=40000") {
		t.Fatalf("unexpected FTP config %q err=%v", ftp, err)
	}
	rsync, err := RenderRsync(values)
	if err != nil || !strings.Contains(rsync, "read only = true") {
		t.Fatalf("unexpected rsync config %q err=%v", rsync, err)
	}

	path := t.TempDir() + "/service.conf"
	if err := os.WriteFile(path, []byte("working\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteValidated(path, "broken\n", func(string) error { return os.ErrInvalid }); err == nil {
		t.Fatal("invalid configuration should not activate")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "working\n" {
		t.Fatalf("working configuration was not preserved: %q err=%v", data, err)
	}
}
