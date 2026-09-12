package shares

import (
	"strings"
	"testing"
)

func TestRenderSambaIncludesTimeMachineOptions(t *testing.T) {
	managed := []ManagedShare{{
		Name: "Backup", Path: "/srv/pools/main/backup", Enabled: true,
		Protocols: []Protocol{{Name: "timemachine"}},
		Access:    []AccessRule{{PrincipalName: "family", Level: "write"}},
	}}
	legacy := []Share{managed[0].Legacy()}
	if !legacy[0].Timemachine {
		t.Fatal("legacy conversion must preserve the Time Machine marker")
	}
	config, err := RenderSamba(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"fruit:model = MacSamba", "vfs objects = catia fruit streams_xattr", "fruit:time machine = yes"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("missing %q in:\n%s", expected, config)
		}
	}
	plain, err := RenderSamba([]Share{{ID: "share-1", Name: "Docs", Path: "/srv/docs", Enabled: true, Protocols: []string{"smb"}, Access: map[string]string{"family": "write"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "fruit") {
		t.Fatalf("non-Time Machine setup must not emit fruit options:\n%s", plain)
	}
}

func TestRenderAvahiPublishesDiskRecords(t *testing.T) {
	values := []ManagedShare{
		{ID: "share-backup", Name: "Backup", Path: "/srv/backup", Enabled: true, Protocols: []Protocol{{Name: "timemachine"}}},
		{ID: "share-docs", Name: "Docs", Path: "/srv/docs", Enabled: true, Protocols: []Protocol{{Name: "smb"}}},
		{ID: "share-off", Name: "Off", Path: "/srv/off", Enabled: false, Protocols: []Protocol{{Name: "timemachine"}}},
	}
	config, err := RenderAvahi(values)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"_smb._tcp", "_adisk._tcp", "sys=waMa=0,adVF=0x100", "dk0=share-backup", "</service-group>"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("missing %q in:\n%s", expected, config)
		}
	}
	if strings.Contains(config, "share-off") {
		t.Fatalf("disabled share must not be announced:\n%s", config)
	}
}

func TestRenderAvahiEmptyWithoutSMBShares(t *testing.T) {
	values := []ManagedShare{{ID: "share-nfs", Name: "NFS", Path: "/srv/nfs", Enabled: true, Protocols: []Protocol{{Name: "nfs"}}}}
	config, err := RenderAvahi(values)
	if err != nil || config != "" {
		t.Fatalf("expected empty announcement, got %q err %v", config, err)
	}
}
