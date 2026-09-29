package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/shares"
)

func TestParseSMBChangeBurstsMatchesConfiguredAuditedShares(t *testing.T) {
	configured := []shares.ManagedShare{
		{ID: "media", Name: "Media", Enabled: true, Protocols: []shares.Protocol{{Name: "smb", Settings: map[string]any{"auditEnabled": true}}}},
		{ID: "private", Name: "Private", Enabled: true, Protocols: []shares.Protocol{{Name: "smb", Settings: map[string]any{"auditEnabled": false}}}},
	}
	var lines []string
	for index := 0; index < smbChangeBurstThreshold; index++ {
		message := fmt.Sprintf("alice|192.0.2.10|Media|unlinkat|ok|docs/file-%d.txt", index)
		encoded, _ := json.Marshal(map[string]string{"MESSAGE": message})
		lines = append(lines, string(encoded))
	}
	lines = append(lines,
		`{"MESSAGE":"alice|192.0.2.10|Private|unlinkat|ok|secret.txt"}`,
		`{"MESSAGE":"alice|192.0.2.10|Media|openat|ok|docs/readme.txt"}`,
		`not-json`,
	)
	bursts := parseSMBChangeBursts([]byte(strings.Join(lines, "\n")), configured)
	if len(bursts) != 1 {
		t.Fatalf("expected only the configured audited share, got %#v", bursts)
	}
	burst := bursts["media\x00alice"]
	if burst.shareID != "media" || burst.actor != "alice" || burst.count != smbChangeBurstThreshold {
		t.Fatalf("unexpected burst: %#v", burst)
	}
}

func TestParseSMBChangeBurstsAcceptsAuditPrefixBeforeOperation(t *testing.T) {
	configured := []shares.ManagedShare{{ID: "docs", Name: "Docs", Enabled: true, Protocols: []shares.Protocol{{Name: "smb", Settings: map[string]any{"auditEnabled": true}}}}}
	bursts := parseSMBChangeBursts([]byte(`{"MESSAGE":"alice|192.0.2.10|Docs|renameat|ok|old|new"}`), configured)
	if len(bursts) != 1 || bursts["docs\x00alice"].count != 1 {
		t.Fatalf("standard Samba full_audit prefix was not parsed: %#v", bursts)
	}
}
