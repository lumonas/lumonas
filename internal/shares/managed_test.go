package shares

import "testing"

func TestManagedShareValidation(t *testing.T) {
	valid := ManagedShare{
		Name: "Documents", Path: "/srv/pool/Documents", Enabled: true,
		Protocols: []Protocol{
			{Name: "smb"},
			{Name: "nfs", Settings: map[string]any{"allowedNetworks": []any{"192.168.1.0/24"}, "rootSquash": true}},
		},
		Access: []AccessRule{{PrincipalName: "family", Level: "write"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid managed share rejected: %v", err)
	}

	cases := []ManagedShare{
		{Name: "Documents", Path: "/srv/pool/Documents", Protocols: []Protocol{{Name: "smb"}, {Name: "smb"}}},
		{Name: "Documents", Path: "/srv/pool/Documents", Protocols: []Protocol{{Name: "nfs", Settings: map[string]any{"allowedNetworks": []any{"not-a-cidr"}}}}},
		{Name: "Documents", Path: "/srv/pool/Documents", Protocols: []Protocol{{Name: "ftp", Settings: map[string]any{"passivePortStart": 21}}}},
		{Name: "Documents", Path: "/srv/pool/Documents", Protocols: []Protocol{{Name: "smb"}}, Access: []AccessRule{{Level: "read"}}},
	}
	for index, value := range cases {
		if err := value.Validate(); err == nil {
			t.Fatalf("case %d unexpectedly passed validation", index)
		}
	}
}

func TestManagedShareLegacyConversion(t *testing.T) {
	value := ManagedShare{
		ID: "share-1", Name: "Media", Path: "/srv/media", Enabled: true, Guest: true,
		Protocols: []Protocol{{Name: "smb"}, {Name: "nfs"}},
		Access:    []AccessRule{{PrincipalName: "family", Level: "read"}},
	}
	legacy := value.Legacy()
	if legacy.ID != value.ID || legacy.Name != value.Name || !legacy.Guest {
		t.Fatalf("legacy metadata mismatch: %#v", legacy)
	}
	if len(legacy.Protocols) != 2 || legacy.Protocols[0] != "smb" || legacy.Access["family"] != "read" {
		t.Fatalf("legacy conversion mismatch: %#v", legacy)
	}
}

func TestManagedShareSMBAuditSettingsAreValidatedAndConverted(t *testing.T) {
	value := ManagedShare{ID: "share-audit", Name: "Records", Path: "/srv/records", Enabled: true, Protocols: []Protocol{{Name: "smb", Settings: map[string]any{"auditEnabled": true, "auditOperations": []any{"renameat", "unlinkat"}}}}}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	legacy := value.Legacy()
	if !legacy.AuditSMB || len(legacy.AuditOps) != 2 || legacy.AuditOps[0] != "renameat" {
		t.Fatalf("SMB audit settings were not converted: %#v", legacy)
	}
	value.Protocols[0].Settings["auditOperations"] = []any{"renameat\\n   include = /etc/passwd"}
	if err := value.Validate(); err == nil {
		t.Fatal("unsafe SMB audit operation was accepted")
	}
}
