package main

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
)

func storageSafetyRequest(target string) request {
	return request{
		Operation:        "filesystem.format",
		OperationID:      "storage-safety-operation",
		PlanHash:         "storage-safety-plan",
		TargetDiskID:     target,
		ExpectedIdentity: map[string]string{"id": target, "serial": "SERIAL-1", "sizeBytes": "100"},
		RequestedState:   map[string]any{"filesystem": "ext4"},
		ExpiresAt:        time.Now().UTC().Add(time.Minute),
		Confirmed:        true,
	}
}

func TestDestructiveStorageExecutionFailsClosedForMissingOrMountedDisk(t *testing.T) {
	disk := model.Disk{ID: "serial:SERIAL-1", Serial: "SERIAL-1", SizeBytes: 100, CurrentPath: "/dev/sda"}
	discover := func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }

	missing := storageSafetyRequest("serial:MISSING")
	result := execute(missing, discover, func(string, ...string) ([]byte, error) {
		t.Fatal("missing disk reached command execution")
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "stable disk identity") {
		t.Fatalf("missing destructive target was not rejected: %#v", result)
	}

	disk.Mounted = true
	mounted := storageSafetyRequest(disk.ID)
	result = execute(mounted, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(string, ...string) ([]byte, error) {
		t.Fatal("mounted disk reached command execution")
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "mounted or assigned") {
		t.Fatalf("mounted destructive target was not rejected: %#v", result)
	}
}

func TestFilesystemMutationValidatesRequestedStateBeforeDiscovery(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		state     map[string]any
		want      string
	}{
		{name: "format requires an allow-listed filesystem", operation: "filesystem.format", want: "filesystem must be ext4 or xfs"},
		{name: "mount requires the canonical branch", operation: "filesystem.mount", state: map[string]any{"filesystem": "ext4", "mountPath": "/srv/pools/media"}, want: "canonical"},
		{name: "erase rejects requested state", operation: "disk.erase", state: map[string]any{"reason": "unsafe"}, want: "does not accept requested state"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := storageSafetyRequest("serial:SERIAL-1")
			req.Operation = test.operation
			req.RequestedState = test.state
			result := execute(req, func(collector.CommandRunner) ([]model.Disk, error) {
				t.Fatal("invalid requested state reached disk discovery")
				return nil, nil
			}, func(string, ...string) ([]byte, error) {
				t.Fatal("invalid requested state reached command execution")
				return nil, nil
			})
			if result.OK || !strings.Contains(result.Error, test.want) {
				t.Fatalf("invalid requested state was not rejected: %#v", result)
			}
		})
	}
}

func TestDestructiveStorageExecutionRejectsKernelPathIdentity(t *testing.T) {
	disk := model.Disk{ID: "path:/dev/sda", CurrentPath: "/dev/sda", SizeBytes: 100}
	result := execute(storageSafetyRequest(disk.ID), func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(string, ...string) ([]byte, error) {
		t.Fatal("unstable disk identity reached command execution")
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "no stable identity") {
		t.Fatalf("kernel-path destructive target was not rejected: %#v", result)
	}
}

func TestDestructiveStorageExecutionFailsClosedWhenFindmntReportsMount(t *testing.T) {
	disk := model.Disk{ID: "serial:SERIAL-1", Serial: "SERIAL-1", SizeBytes: 100, CurrentPath: "/dev/sda"}
	var commands []string
	result := execute(storageSafetyRequest(disk.ID), func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, _ ...string) ([]byte, error) {
		commands = append(commands, name)
		if name == "findmnt" {
			return []byte("/dev/sda /srv/disks/serial_SERIAL-1 ext4 rw\n"), nil
		}
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "device or one of its partitions is mounted") {
		t.Fatalf("findmnt-reported mount was not rejected: %#v", result)
	}
	if len(commands) != 1 || commands[0] != "findmnt" {
		t.Fatalf("destructive command ran before mount safety check: %#v", commands)
	}
}

func TestDestructiveStorageExecutionDetectsMountedPartitionWithLsblk(t *testing.T) {
	disk := model.Disk{ID: "serial:SERIAL-1", Serial: "SERIAL-1", SizeBytes: 100, CurrentPath: "/dev/sda"}
	result := execute(storageSafetyRequest(disk.ID), func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, _ ...string) ([]byte, error) {
		if name == "lsblk" {
			return []byte("\n/srv/pools/main\n"), nil
		}
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "device or one of its partitions is mounted") {
		t.Fatalf("mounted partition was not rejected by lsblk fallback: %#v", result)
	}
}
