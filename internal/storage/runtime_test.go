package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestParseMounts(t *testing.T) {
	items := parseMounts("/srv/pools/main fuse.mergerfs /srv/disks/wwn-a:/srv/disks/wwn-b\n")
	if len(items) != 1 || items[0].Target != "/srv/pools/main" || items[0].Source != "/srv/disks/wwn-a:/srv/disks/wwn-b" {
		t.Fatalf("unexpected mounts: %#v", items)
	}
}

func TestDiscoverPoolsUsesStableDiskIDs(t *testing.T) {
	disks := []model.Disk{{ID: "wwn-a", CurrentPath: "/dev/sda"}, {ID: "wwn-b", CurrentPath: "/dev/sdb"}}
	runner := func(_ context.Context, command string, args ...string) ([]byte, error) {
		if command != "findmnt" {
			t.Fatalf("unexpected command: %s %v", command, args)
		}
		return []byte("/tmp fuse.mergerfs /srv/disks/wwn-b:/srv/disks/wwn-a\n"), nil
	}
	items := DiscoverPools(context.Background(), disks, runner)
	if len(items) != 1 || len(items[0].Members) != 2 || items[0].Members[0].DiskID != "wwn-b" {
		t.Fatalf("unexpected pools: %#v", items)
	}
}

func TestDiscoverProtectionReadsConfigWithoutMutating(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "snapraid.conf")
	if err := os.WriteFile(config, []byte("parity /srv/disks/wwn:parity/parity\ndata data-a /srv/disks/wwn:data\ncontent /var/lib/lumonas/content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	disks := []model.Disk{{ID: "wwn:parity", SizeBytes: 10}, {ID: "wwn:data", SizeBytes: 20}}
	calls := 0
	runner := func(_ context.Context, command string, args ...string) ([]byte, error) {
		calls++
		return []byte("status"), nil
	}
	result := DiscoverProtection(context.Background(), disks, runner, config)
	if result.Status != model.Attention || len(result.ParityDisks) != 1 || len(result.ProtectedDiskIDs) != 1 || calls != 1 {
		t.Fatalf("unexpected protection: %#v calls=%d", result, calls)
	}
}

func TestDiscoverProtectionMarksMissingConfiguredDiskCritical(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "snapraid.conf")
	if err := os.WriteFile(config, []byte("parity /srv/disks/wwn-parity/parity\ndata data-a /srv/disks/wwn-data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := DiscoverProtection(context.Background(), []model.Disk{{ID: "wwn-data"}}, func(context.Context, string, ...string) ([]byte, error) { return nil, nil }, config)
	if result.Status != model.Critical {
		t.Fatalf("expected critical missing-disk state: %#v", result)
	}
}
