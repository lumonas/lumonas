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
