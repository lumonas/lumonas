package shares

import (
	"strings"
	"testing"
)

func TestRenderSambaEnablesTimeMachineOnlyForEnabledTimeMachineShares(t *testing.T) {
	config, err := RenderSamba([]Share{
		{Name: "Backup", Path: "/srv/backup", Enabled: true, Protocols: []string{"smb"}, Timemachine: true},
		{Name: "Disabled", Path: "/srv/disabled", Enabled: false, Protocols: []string{"smb"}, Timemachine: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"fruit:model = MacSamba", "vfs objects = catia fruit streams_xattr shadow_copy2", "fruit:time machine = yes", "fruit:time machine max size = 0", "shadow:snapdir = /srv/backup.snapshots", "shadow:basedir = /srv/backup", "shadow:format = %Y.%m.%d-%H.%M.%S"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("Time Machine config missing %q:\n%s", expected, config)
		}
	}
}

func TestRenderSambaAddsFilteredFullAuditWithoutRemovingTimeMachineModules(t *testing.T) {
	config, err := RenderSamba([]Share{{Name: "Records", Path: "/srv/records", Enabled: true, Protocols: []string{"smb"}, Timemachine: true, AuditSMB: true, AuditOps: []string{"renameat", "unlinkat"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"vfs objects = catia fruit streams_xattr shadow_copy2 full_audit", "full_audit:prefix = %u|%I|%S", "full_audit:success = renameat unlinkat", "full_audit:failure = none", "fruit:time machine = yes"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("SMB auditing config missing %q:\n%s", expected, config)
		}
	}
}
