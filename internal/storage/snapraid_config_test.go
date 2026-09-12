package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestRenderSnapraidConfigIsStableAndCanonical(t *testing.T) {
	first, err := RenderSnapraidConfig("wwn:p", []string{"wwn:b", "wwn:a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderSnapraidConfig("wwn:p", []string{"wwn:a", "wwn:b"})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("config is not stable across input order:\n%s\n---\n%s", first, second)
	}
	expected := strings.Join([]string{
		"# " + UnitDirectoryHint,
		"parity /srv/disks/wwn_p/snapraid.parity",
		"content /var/lib/lumonas/snapraid.content",
		"content /srv/disks/wwn_a/snapraid.content",
		"content /srv/disks/wwn_b/snapraid.content",
		"data d1 /srv/disks/wwn_a",
		"data d2 /srv/disks/wwn_b",
		"",
	}, "\n")
	if first != expected {
		t.Fatalf("unexpected config:\n%s\nexpected:\n%s", first, expected)
	}
}

func TestRenderSnapraidConfigRejectsUnsafeLayouts(t *testing.T) {
	if _, err := RenderSnapraidConfig("wwn:p", nil); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected data disk requirement: %v", err)
	}
	if _, err := RenderSnapraidConfig("wwn:a", []string{"wwn:a"}); err == nil || !strings.Contains(err.Error(), "cannot also be") {
		t.Fatalf("expected parity/data overlap rejection: %v", err)
	}
	if _, err := RenderSnapraidConfig("wwn:p", []string{"wwn:a", "wwn:a"}); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected duplicate rejection: %v", err)
	}
	if _, err := RenderSnapraidConfig("wwn:p", []string{""}); err == nil {
		t.Fatal("expected empty identity rejection")
	}
}

func TestValidateSnapraidConfigRejectsUnmanagedDirectives(t *testing.T) {
	valid, err := RenderSnapraidConfig("wwn:p", []string{"wwn:a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSnapraidConfig(valid); err != nil {
		t.Fatalf("generated config should validate: %v", err)
	}
	if err := ValidateSnapraidConfig("data d1 /srv/disks/wwn_a\nexec rm -rf /\n"); err == nil {
		t.Fatal("expected unmanaged directive rejection")
	}
}

func TestDiscoverProtectionParsesGeneratedConfig(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "snapraid.conf")
	config, err := RenderSnapraidConfig("wwn:p", []string{"wwn:b", "wwn:a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	disks := []model.Disk{{ID: "wwn:a", WWN: "5000cca-aaaa", SizeBytes: 100}, {ID: "wwn:b", WWN: "5000cca-bbbb", SizeBytes: 200}}
	result := DiscoverProtection(context.Background(), disks, func(context.Context, string, ...string) ([]byte, error) { return nil, nil }, configPath)
	// The parity disk wwn:p is absent, so the missing configured disk is critical.
	if result.Status != model.Critical || len(result.ParityDisks) != 0 {
		t.Fatalf("unexpected protection: %#v", result)
	}
	if len(result.ProtectedDiskIDs) != 2 {
		t.Fatalf("expected both data disks protected: %#v", result.ProtectedDiskIDs)
	}
	parityDisk := model.Disk{ID: "wwn:p", WWN: "5000cca-pppp", SizeBytes: 300}
	disks = append(disks, parityDisk)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	result = DiscoverProtection(context.Background(), disks, func(context.Context, string, ...string) ([]byte, error) { return nil, nil }, configPath)
	if result.Status != model.Attention || len(result.ParityDisks) != 1 || result.ParityDisks[0].DiskID != "wwn:p" || len(result.ProtectedDiskIDs) != 2 {
		t.Fatalf("unexpected protection for full layout: %#v", result)
	}
}
