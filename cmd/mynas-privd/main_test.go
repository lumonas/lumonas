package main

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
)

func TestPrivilegedProtocolRejectsUnknownOperation(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	go serve(server)
	if _, err := client.Write([]byte(`{"operation":"shell.exec","planHash":"confirmed"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	var response response
	if err := json.NewDecoder(bufio.NewReader(client)).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == "" {
		t.Fatalf("expected rejected operation, got %#v", response)
	}
}

func TestPrivilegedProtocolRequiresConfirmationForTypedOperation(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	go serve(server)
	if _, err := client.Write([]byte(`{"operation":"filesystem.create","planHash":"confirmed","targetDiskId":"wwn:test"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	var result response
	if err := json.NewDecoder(bufio.NewReader(client)).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "operation plan is not confirmed" {
		t.Fatalf("unexpected response %#v", result)
	}
}

func TestExecuteMountRevalidatesIdentityAndUsesAllowListedCommand(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100}
	var command string
	run := func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	}
	result := execute(request{Operation: "filesystem.mount", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "test", "sizeBytes": "100"}, RequestedState: map[string]any{"mountPath": "/srv/disks/wwn-test", "filesystem": "ext4"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, run)
	if !result.OK || command != "mount -t ext4 /dev/sda /srv/disks/wwn-test" {
		t.Fatalf("unexpected result: %#v command=%q", result, command)
	}
}

func TestExecuteRejectsStaleIdentity(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "new"}
	result := execute(request{Operation: "disk.erase", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "old"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(string, ...string) ([]byte, error) { return nil, nil })
	if result.OK || !strings.Contains(result.Error, "identity mismatch") {
		t.Fatalf("expected stale identity rejection: %#v", result)
	}
}

func TestExecuteRejectsMountedPartitionForDestructiveAction(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", SizeBytes: 100}
	result := execute(request{Operation: "filesystem.format", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"sizeBytes": "100"}, RequestedState: map[string]any{"filesystem": "ext4"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, _ ...string) ([]byte, error) {
		if name == "findmnt" {
			return []byte("/srv/pools/main"), nil
		}
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "mounted") {
		t.Fatalf("expected mounted partition rejection: %#v", result)
	}
}
