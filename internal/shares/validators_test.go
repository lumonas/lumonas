package shares

import (
	"os"
	"path/filepath"
	"testing"
)

func writeValidatorFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProtocolConfigValidatorsAcceptGeneratedFixtures(t *testing.T) {
	if err := ValidateNFSExports(writeValidatorFixture(t, "exports", "/srv/media 192.168.1.0/24(rw,sync,root_squash)\n")); err != nil {
		t.Fatalf("NFS fixture rejected: %v", err)
	}
	if err := ValidateSFTPConfig(writeValidatorFixture(t, "sftp.conf", "Subsystem sftp internal-sftp\nMatch User family\n    ChrootDirectory /srv/media\n    ForceCommand internal-sftp -R\n    X11Forwarding no\n    AllowTcpForwarding no\n")); err != nil {
		t.Fatalf("SFTP fixture rejected: %v", err)
	}
	ftp := "listen=YES\nanonymous_enable=NO\nlocal_enable=YES\nchroot_local_user=YES\nsecure_chroot_dir=/var/run/vsftpd/empty\nuser_config_dir=/var/lib/lumonas/generated/vsftpd-users\npasv_min_port=50000\npasv_max_port=51000\nssl_enable=YES\nrsa_cert_file=/etc/lumonas/tls/server.crt\nrsa_private_key_file=/etc/lumonas/tls/server.key\n"
	if err := ValidateFTPConfig(writeValidatorFixture(t, "ftp.conf", ftp)); err != nil {
		t.Fatalf("FTP fixture rejected: %v", err)
	}
	if err := ValidateFTPUserConfig(writeValidatorFixture(t, "family", "local_root=/srv/media\nwrite_enable=NO\n")); err != nil {
		t.Fatalf("FTP user fixture rejected: %v", err)
	}
	if err := ValidateRsyncConfig(writeValidatorFixture(t, "rsync.conf", "uid = nobody\ngid = nogroup\nuse chroot = yes\n[media]\npath = /srv/media\nread only = true\n")); err != nil {
		t.Fatalf("rsync fixture rejected: %v", err)
	}
}

func TestProtocolConfigValidatorsRejectUnsafeFixtures(t *testing.T) {
	cases := []struct {
		name  string
		check func(string) error
		body  string
	}{
		{"nfs", ValidateNFSExports, "/srv/media 192.168.1.0/24(rw\n"},
		{"sftp", ValidateSFTPConfig, "Subsystem sftp internal-sftp\nChrootDirectory /srv/../etc\n"},
		{"ftp", ValidateFTPConfig, "listen=YES\nanonymous_enable=YES\nlocal_enable=YES\nsecure_chroot_dir=/var/run/vsftpd/empty\nuser_config_dir=/var/lib/lumonas/users\npasv_min_port=50000\npasv_max_port=51000\n"},
		{"rsync", ValidateRsyncConfig, "[media]\npath = relative\nread only = true\n"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.check(writeValidatorFixture(t, test.name, test.body)); err == nil {
				t.Fatal("unsafe fixture was accepted")
			}
		})
	}
}
