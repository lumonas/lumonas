package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

func TestApplyMountPersistenceWritesEnablesAndPrunes(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LUMONAS_UNIT_DIR", directory)
	stale := filepath.Join(directory, "srv-disks-old.mount")
	if err := os.WriteFile(stale, []byte("[Mount]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(directory, "sshd.service")
	if err := os.WriteFile(unrelated, []byte("[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commands := make([]string, 0)
	entries := []map[string]any{
		{"kind": "disk", "targetId": "wwn:a", "mountPath": "/srv/disks/wwn_a", "fstype": "ext4", "source": "UUID=abc-123", "options": storage.DiskMountOptions, "enabled": true},
		{"kind": "pool", "targetId": "media", "mountPath": "/srv/pools/media", "fstype": "fuse.mergerfs", "source": "/srv/disks/wwn_a", "options": storage.PoolMountOptions, "enabled": true},
	}
	result := applyMountPersistence(request{Operation: "storage.mountpersist.apply", PlanHash: "mountpersist-1", RequestedState: map[string]any{"entries": entriesOf(entries)}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(directory, "srv-disks-wwn_a.mount"))
	if err != nil || !strings.Contains(string(content), "What=UUID=abc-123") {
		t.Fatalf("disk unit missing: %v %s", err, string(content))
	}
	poolContent, err := os.ReadFile(filepath.Join(directory, "srv-pools-media.mount"))
	if err != nil || !strings.Contains(string(poolContent), "Requires=srv-disks-wwn_a.mount") {
		t.Fatalf("pool unit missing: %v %s", err, string(poolContent))
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale unit was not pruned: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated unit was removed: %v", err)
	}
	joined := strings.Join(commands, "; ")
	if !strings.Contains(joined, "systemctl daemon-reload") || !strings.Contains(joined, "systemctl enable srv-disks-wwn_a.mount") || !strings.Contains(joined, "systemctl enable srv-pools-media.mount") {
		t.Fatalf("unexpected systemctl commands: %v", commands)
	}
}

func TestApplyMountPersistencePrunesEverythingOnEmptyState(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LUMONAS_UNIT_DIR", directory)
	stale := filepath.Join(directory, "srv-pools-media.mount")
	if err := os.WriteFile(stale, []byte("[Mount]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commands := make([]string, 0)
	result := applyMountPersistence(request{Operation: "storage.mountpersist.apply", PlanHash: "mountpersist-2", RequestedState: map[string]any{"entries": []any{}}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale pool unit was not pruned: %v", err)
	}
	if !strings.Contains(strings.Join(commands, "; "), "systemctl daemon-reload") {
		t.Fatalf("expected daemon-reload: %v", commands)
	}
}

func TestApplyMountPersistenceRequiresConfirmationAndSafeState(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LUMONAS_UNIT_DIR", directory)
	unconfirmed := applyMountPersistence(request{Operation: "storage.mountpersist.apply", PlanHash: "mountpersist-3", RequestedState: map[string]any{"entries": []any{}}}, func(string, ...string) ([]byte, error) { return nil, nil })
	if unconfirmed.OK || !strings.Contains(unconfirmed.Error, "not confirmed") {
		t.Fatalf("expected confirmation gate: %#v", unconfirmed)
	}
	unsafe := applyMountPersistence(request{Operation: "storage.mountpersist.apply", PlanHash: "mountpersist-4", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute), RequestedState: map[string]any{"entries": []any{map[string]any{"kind": "disk", "targetId": "evil", "mountPath": "/etc/cron.d/evil", "fstype": "ext4", "source": "UUID=x", "enabled": true}}}}, func(string, ...string) ([]byte, error) { return nil, nil })
	if unsafe.OK || !strings.Contains(unsafe.Error, "canonical") {
		t.Fatalf("expected unsafe path rejection: %#v", unsafe)
	}
	if items, err := os.ReadDir(directory); err != nil || len(items) != 0 {
		t.Fatalf("no unit should be written for rejected state: %v %v", items, err)
	}
}

func entriesOf(entries []map[string]any) []any {
	result := make([]any, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result
}
