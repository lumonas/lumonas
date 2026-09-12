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
	for _, expected := range []string{"fruit:model = MacSamba", "vfs objects = catia fruit streams_xattr", "fruit:time machine = yes", "fruit:time machine max size = 0"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("Time Machine config missing %q:\n%s", expected, config)
		}
	}
}
