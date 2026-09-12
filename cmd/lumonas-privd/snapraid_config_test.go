package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/storage"
)

func TestApplySnapraidConfigWritesCanonicalLayout(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "snapraid.conf")
	commands := make([]string, 0)
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	content, err := storage.RenderSnapraidConfig("wwn:p", []string{"wwn:b", "wwn:a"})
	if err != nil {
		t.Fatal(err)
	}
	result := activateSnapraidConfig(configPath, content, run)
	if !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "parity /srv/disks/wwn_p/snapraid.parity") || !strings.Contains(string(written), "data d1 /srv/disks/wwn_a") || !strings.Contains(string(written), "data d2 /srv/disks/wwn_b") {
		t.Fatalf("unexpected config content:\n%s", written)
	}
	validated := false
	for _, command := range commands {
		if strings.HasPrefix(command, "snapraid -c ") && strings.HasSuffix(command, " status") {
			validated = true
		}
	}
	if !validated {
		t.Fatalf("generated config was not validated before activation: %v", commands)
	}
	repeat := activateSnapraidConfig(configPath, content, run)
	if !repeat.OK || repeat.Data.(map[string]string)["state"] != "unchanged" {
		t.Fatalf("expected unchanged state on repeat: %#v", repeat)
	}
}

func TestApplySnapraidConfigRejectsUnsafePathsAndLayouts(t *testing.T) {
	base := request{Operation: "snapraid.config.apply", PlanHash: "protection-3", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	outside := applySnapraidConfig(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, ExpiresAt: base.ExpiresAt, RequestedState: map[string]any{"configPath": "/etc/passwd", "dataDiskIds": []any{"wwn:a"}}}, nil, nil)
	if outside.OK || !strings.Contains(outside.Error, "allow-listed") {
		t.Fatalf("expected config path rejection: %#v", outside)
	}
	unconfirmed := applySnapraidConfig(request{Operation: base.Operation, PlanHash: base.PlanHash, ExpiresAt: base.ExpiresAt, RequestedState: map[string]any{"configPath": "/etc/lumonas/snapraid.conf", "dataDiskIds": []any{"wwn:a"}}}, nil, nil)
	if unconfirmed.OK || !strings.Contains(unconfirmed.Error, "not confirmed") {
		t.Fatalf("expected confirmation gate: %#v", unconfirmed)
	}
	overlap := applySnapraidConfig(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, ExpiresAt: base.ExpiresAt, RequestedState: map[string]any{"configPath": "/etc/lumonas/snapraid.conf", "parityDiskId": "wwn:a", "dataDiskIds": []any{"wwn:a"}}}, nil, nil)
	if overlap.OK || !strings.Contains(overlap.Error, "cannot also be") {
		t.Fatalf("expected layout rejection: %#v", overlap)
	}
}

func TestApplySnapraidConfigRevalidatesDiskIdentities(t *testing.T) {
	request := request{Operation: "snapraid.config.apply", PlanHash: "hash", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute), ExpectedDisks: []expectedDisk{{ID: "wwn:a", WWN: "a", SizeBytes: 100}}, RequestedState: map[string]any{"configPath": "/etc/lumonas/snapraid.conf", "dataDiskIds": []string{"wwn:a"}}}
	result := applySnapraidConfig(request, func(_ collector.CommandRunner) ([]model.Disk, error) {
		return []model.Disk{{ID: "wwn:a", WWN: "changed", SizeBytes: 100}}, nil
	}, func(string, ...string) ([]byte, error) { return nil, nil })
	if result.OK || !strings.Contains(result.Error, "identity mismatch") {
		t.Fatalf("expected stale identity rejection: %#v", result)
	}
}

func TestRequestedStringsAcceptsJSONAndTypedSlices(t *testing.T) {
	if got := requestedStrings(map[string]any{"ids": []any{"wwn:a", "wwn:b"}}, "ids"); strings.Join(got, ",") != "wwn:a,wwn:b" {
		t.Fatalf("unexpected JSON slice: %#v", got)
	}
	if got := requestedStrings(map[string]any{"ids": []string{"wwn:a", "wwn:b"}}, "ids"); strings.Join(got, ",") != "wwn:a,wwn:b" {
		t.Fatalf("unexpected typed slice: %#v", got)
	}
}
