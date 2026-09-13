package shares

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzValidateGeneratedShareConfigs(f *testing.F) {
	f.Add("/srv/media 192.168.1.0/24(rw,sync,root_squash)\n")
	f.Add("Subsystem sftp internal-sftp\nMatch User family\nChrootDirectory /srv/media\n")
	f.Add("[media]\npath = /srv/media\nread only = true\n")
	f.Fuzz(func(t *testing.T, content string) {
		if len(content) > maxGeneratedConfigBytes {
			t.Skip("fixture exceeds generated configuration bound")
		}
		path := filepath.Join(t.TempDir(), "generated.conf")
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
		_ = ValidateNFSExports(path)
		_ = ValidateSFTPConfig(path)
		_ = ValidateFTPConfig(path)
		_ = ValidateFTPUserConfig(path)
		_ = ValidateRsyncConfig(path)
	})
}

func FuzzManagedShareValidation(f *testing.F) {
	f.Add("media", "/srv/media", "smb", true)
	f.Add("", "relative", "unknown", false)
	f.Fuzz(func(t *testing.T, name, path, protocol string, guest bool) {
		share := ManagedShare{
			ID: "fuzz-share", Name: name, Path: path, Enabled: true, Guest: guest,
			Protocols: []Protocol{{Name: protocol}},
		}
		_ = share.Validate()
	})
}
