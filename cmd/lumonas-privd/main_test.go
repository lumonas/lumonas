package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
	commandrunner "github.com/lumonas/lumonas/internal/runner"
)

func TestCommandRunnerKillsDescendantsAfterTimeout(t *testing.T) {
	previousTimeout := privilegedCommandTimeout
	privilegedCommandTimeout = 30 * time.Millisecond
	defer func() { privilegedCommandTimeout = previousTimeout }()

	started := time.Now()
	_, err := commandRunner("sh", "-c", "sleep 10 & wait")
	if err == nil {
		t.Fatal("expected privileged command timeout")
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("timed-out command retained a descendant: elapsed=%s err=%v", elapsed, err)
	}
}

func TestCommandRunnerUsesCallerCancellation(t *testing.T) {
	previousTimeout := privilegedCommandTimeout
	privilegedCommandTimeout = time.Minute
	defer func() { privilegedCommandTimeout = previousTimeout }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := commandrunner.CombinedOutputContext(ctx, "sh", "-c", "sleep 10 & wait")
	if err == nil {
		t.Fatal("expected caller cancellation")
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("caller cancellation retained a descendant: elapsed=%s err=%v", elapsed, err)
	}
}

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

func TestExecuteRequiresOperationIDForConfirmedStorageMutation(t *testing.T) {
	result := execute(request{Operation: "filesystem.format", PlanHash: "plan", TargetDiskID: "wwn:test", Confirmed: true}, nil, func(string, ...string) ([]byte, error) {
		t.Fatal("storage command ran without an operation ID")
		return nil, nil
	})
	if result.OK || result.Error != "operationId is required" {
		t.Fatalf("unexpected operation-id response: %#v", result)
	}
}

func TestExecuteMountRevalidatesIdentityAndUsesAllowListedCommand(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", GPTDiskGUID: "gpt-test", PartitionUUID: "part-test", SizeBytes: 100}
	var command string
	run := func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	}
	result := execute(request{Operation: "filesystem.mount", OperationID: "op-mount", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "test", "gptDiskGuid": "gpt-test", "partitionUuid": "part-test", "sizeBytes": "100"}, RequestedState: map[string]any{"mountPath": "/srv/disks/wwn-test", "filesystem": "ext4"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, run)
	if !result.OK || command != "mount -t ext4 /dev/sda /srv/disks/wwn-test" {
		t.Fatalf("unexpected result: %#v command=%q", result, command)
	}
}

func TestExecuteCreateFormatsLabelsAndMounts(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100}
	commands := make([]string, 0)
	result := execute(request{Operation: "filesystem.create", OperationID: "op-create", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "test", "sizeBytes": "100"}, RequestedState: map[string]any{"filesystem": "ext4", "mountPath": "/srv/disks/wwn-test", "label": "media"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(commands) != 5 || commands[0] != "findmnt -rn -S /dev/sda" || commands[1] != "lsblk -nrpo MOUNTPOINT -- /dev/sda" || commands[2] != "mkfs.ext4 -F -L media /dev/sda" || commands[3] != "mkdir -p /srv/disks/wwn-test" || commands[4] != "mount -t ext4 /dev/sda /srv/disks/wwn-test" {
		t.Fatalf("unexpected commands: %v", commands)
	}
}

func TestEncryptedCreatePassesPassphraseOnlyOverStdin(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100}
	passphrase := "correct horse battery staple"
	var commands []string
	var secretInputs []string
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	runStdin := func(name string, args []string, stdin string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		secretInputs = append(secretInputs, stdin)
		return nil, nil
	}
	result := executeStorageWithStdin(request{Operation: "filesystem.create", RequestedState: map[string]any{
		"filesystem": "ext4", "mountPath": "/srv/disks/wwn-test", "encrypted": true, "encryptionPassphrase": passphrase,
	}}, disk, run, runStdin)
	if !result.OK || strings.Contains(result.Error, passphrase) {
		t.Fatalf("encrypted format failed or exposed secret: %#v", result)
	}
	mapper := cryptoMapperName(disk.ID)
	if len(secretInputs) != 2 || secretInputs[0] != passphrase || secretInputs[1] != passphrase {
		t.Fatalf("expected passphrase to be supplied to both cryptsetup commands over stdin: %#v", secretInputs)
	}
	want := []string{
		"cryptsetup luksFormat --batch-mode --type luks2 --key-file - /dev/sda",
		"cryptsetup open --key-file - /dev/sda " + mapper,
		"mkfs.ext4 -F /dev/mapper/" + mapper,
		"mkdir -p /srv/disks/wwn-test",
		"mount -t ext4 /dev/mapper/" + mapper + " /srv/disks/wwn-test",
	}
	if strings.Join(commands, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected encrypted format commands:\n%v", commands)
	}
}

func TestExecuteAllowsTransientPassphraseForEncryptedCreate(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100}
	previousRunner := cryptoCommandRunner
	t.Cleanup(func() { cryptoCommandRunner = previousRunner })
	var received int
	cryptoCommandRunner = func(_ string, _ []string, stdin string) ([]byte, error) {
		if stdin != "correct horse battery staple" {
			t.Fatal("transient passphrase did not reach cryptsetup stdin")
		}
		received++
		return nil, nil
	}
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	result := execute(request{
		Operation: "filesystem.create", OperationID: "op-encrypted", PlanHash: "hash", TargetDiskID: disk.ID,
		ExpectedIdentity: map[string]string{"wwn": "test", "sizeBytes": "100"},
		RequestedState:   map[string]any{"filesystem": "ext4", "mountPath": "/srv/disks/wwn-test", "encrypted": true, "encryptionPassphrase": "correct horse battery staple"},
		ExpiresAt:        time.Now().UTC().Add(time.Minute), Confirmed: true,
	}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, run)
	if !result.OK || received != 2 {
		t.Fatalf("encrypted create did not pass worker validation: result=%#v stdinCalls=%d", result, received)
	}
}

func TestExecuteCreateEnforcesCanonicalPathFilesystemAndLabel(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100}
	discover := func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	base := request{Operation: "filesystem.create", OperationID: "op-create", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "test"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}
	cases := []struct {
		name     string
		request  map[string]any
		expected string
	}{
		{"foreign mount path", map[string]any{"filesystem": "ext4", "mountPath": "/srv/disks/other"}, "canonical"},
		{"unsupported filesystem", map[string]any{"filesystem": "btrfs", "mountPath": "/srv/disks/wwn-test"}, "ext4 or xfs"},
		{"invalid label", map[string]any{"filesystem": "ext4", "mountPath": "/srv/disks/wwn-test", "label": "-bad label"}, "label is invalid"},
	}
	for _, testCase := range cases {
		result := execute(request{Operation: base.Operation, OperationID: base.OperationID, PlanHash: base.PlanHash, TargetDiskID: base.TargetDiskID, ExpectedIdentity: base.ExpectedIdentity, RequestedState: testCase.request, ExpiresAt: base.ExpiresAt, Confirmed: true}, discover, run)
		if result.OK || !strings.Contains(result.Error, testCase.expected) {
			t.Fatalf("%s: expected rejection %q, got %#v", testCase.name, testCase.expected, result)
		}
	}
}

func TestExecuteCreateRejectsDestructiveTargets(t *testing.T) {
	mounted := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100, Mounted: true}
	result := execute(request{Operation: "filesystem.create", OperationID: "op-create-mounted", PlanHash: "hash", TargetDiskID: mounted.ID, ExpectedIdentity: map[string]string{"wwn": "test"}, RequestedState: map[string]any{"filesystem": "ext4", "mountPath": "/srv/disks/wwn-test"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{mounted}, nil }, func(string, ...string) ([]byte, error) { return nil, nil })
	if result.OK || !strings.Contains(result.Error, "mounted") {
		t.Fatalf("expected mounted target rejection: %#v", result)
	}
}

func TestExecuteMountCanBeReadOnlyForImport(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100}
	var command string
	result := execute(request{Operation: "filesystem.mount", OperationID: "op-readonly", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "test", "sizeBytes": "100"}, RequestedState: map[string]any{"mountPath": "/srv/disks/wwn-test", "filesystem": "xfs", "readOnly": true}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	if !result.OK || command != "mount -t xfs -o ro /dev/sda /srv/disks/wwn-test" {
		t.Fatalf("unexpected read-only mount: %#v command=%q", result, command)
	}
}

func TestExecuteRejectsStaleIdentity(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "new"}
	result := execute(request{Operation: "disk.erase", OperationID: "op-erase", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "old"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(string, ...string) ([]byte, error) { return nil, nil })
	if result.OK || !strings.Contains(result.Error, "identity mismatch") {
		t.Fatalf("expected stale identity rejection: %#v", result)
	}
}

func TestExecuteRejectsMountedPartitionForDestructiveAction(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", SizeBytes: 100}
	result := execute(request{Operation: "filesystem.format", OperationID: "op-format", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"sizeBytes": "100"}, RequestedState: map[string]any{"filesystem": "ext4"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, _ ...string) ([]byte, error) {
		if name == "lsblk" {
			return []byte("\n/srv/pools/main\n"), nil
		}
		if name == "findmnt" {
			return []byte("/srv/pools/main"), nil
		}
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "mounted") {
		t.Fatalf("expected mounted partition rejection: %#v", result)
	}
}

func TestExecuteFailsClosedWhenMountStateCannotBeVerified(t *testing.T) {
	disk := model.Disk{ID: "wwn-test", CurrentPath: "/dev/sda", WWN: "test", SizeBytes: 100}
	result := execute(request{Operation: "disk.erase", OperationID: "op-verify-mount", PlanHash: "hash", TargetDiskID: disk.ID, ExpectedIdentity: map[string]string{"wwn": "test", "sizeBytes": "100"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, _ ...string) ([]byte, error) {
		if name == "findmnt" || name == "lsblk" {
			return nil, errors.New("mount lookup unavailable")
		}
		t.Fatal("destructive command ran without mount-state verification")
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "verify target mount state") {
		t.Fatalf("expected fail-closed mount verification error: %#v", result)
	}
}

func TestExecuteSnapraidUsesAllowListedConfigAndPercent(t *testing.T) {
	var command string
	result := execute(request{Operation: "snapraid.scrub", OperationID: "job-1", PlanHash: "job-1", RequestedState: map[string]any{"configPath": "/etc/lumonas/snapraid.conf", "scrubPercent": "10"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, nil, func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	if !result.OK || command != "snapraid -c /etc/lumonas/snapraid.conf scrub -p 10" {
		t.Fatalf("unexpected snapraid execution: %#v command=%q", result, command)
	}
}

func TestExecutePowerActionIsAllowListed(t *testing.T) {
	command := ""
	result := execute(request{Operation: "power.action", OperationID: "power-1", PlanHash: "power-1", RequestedState: map[string]any{"action": "reboot"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, nil, func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	if !result.OK || command != "systemctl reboot" {
		t.Fatalf("unexpected power action: %#v command=%q", result, command)
	}
}

func TestExecutePoolMountRevalidatesEveryDisk(t *testing.T) {
	disks := []model.Disk{{ID: "wwn:a", WWN: "a", SizeBytes: 100}, {ID: "wwn:b", WWN: "b", SizeBytes: 200}}
	var command string
	result := execute(request{Operation: "pool.mount", OperationID: "pool-1", PlanHash: "pool-1", ExpectedDisks: []expectedDisk{{ID: "wwn:a", WWN: "a", SizeBytes: 100}, {ID: "wwn:b", WWN: "b", SizeBytes: 200}}, RequestedState: map[string]any{"mountPath": "/srv/pools/media", "branches": []any{"/srv/disks/wwn_a", "/srv/disks/wwn_b"}}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return disks, nil }, func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		if name == "findmnt" && len(args) > 0 && strings.HasPrefix(args[len(args)-1], "/srv/disks/") {
			return []byte("/dev/sdX"), nil
		}
		return nil, nil
	})
	if !result.OK || !strings.Contains(command, "mount -t fuse.mergerfs") {
		t.Fatalf("unexpected pool mount result=%#v command=%q", result, command)
	}
	changed := []model.Disk{{ID: "wwn:a", WWN: "a", SizeBytes: 100}, {ID: "wwn:b", WWN: "replaced", SizeBytes: 200}}
	result = execute(request{Operation: "pool.mount", OperationID: "pool-1", PlanHash: "pool-1", ExpectedDisks: []expectedDisk{{ID: "wwn:a", WWN: "a", SizeBytes: 100}, {ID: "wwn:b", WWN: "b", SizeBytes: 200}}, RequestedState: map[string]any{"mountPath": "/srv/pools/media", "branches": []any{"/srv/disks/wwn_a", "/srv/disks/wwn_b"}}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return changed, nil }, func(name string, args ...string) ([]byte, error) {
		if name == "findmnt" && len(args) > 0 && strings.HasPrefix(args[len(args)-1], "/srv/disks/") {
			return []byte("/dev/sdX"), nil
		}
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "WWN mismatch") {
		t.Fatalf("expected pool identity rejection: %#v", result)
	}
}

func TestExecutePoolMountRejectsNewCriticalDisk(t *testing.T) {
	disk := model.Disk{ID: "wwn:a", WWN: "a", SizeBytes: 100, Health: model.Critical}
	result := execute(request{Operation: "pool.mount", OperationID: "pool-1", PlanHash: "pool-1", ExpectedDisks: []expectedDisk{{ID: "wwn:a", WWN: "a", SizeBytes: 100}}, RequestedState: map[string]any{"mountPath": "/srv/pools/media", "branches": []any{"/srv/disks/wwn_a"}}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, func(collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, args ...string) ([]byte, error) {
		if name == "findmnt" && len(args) > 0 && strings.HasPrefix(args[len(args)-1], "/srv/disks/") {
			return []byte("/dev/sdX"), nil
		}
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "critically unhealthy") {
		t.Fatalf("expected critical disk rejection: %#v", result)
	}
}

func TestExecutePoolUnmountOnlyAllowsMergerfsMounts(t *testing.T) {
	commands := make([]string, 0)
	result := execute(request{Operation: "pool.unmount", OperationID: "pool-unmount", PlanHash: "pool-unmount", RequestedState: map[string]any{"mountPath": "/srv/pools/media"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, nil, func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if name == "findmnt" {
			return []byte("fuse.mergerfs"), nil
		}
		return nil, nil
	})
	if !result.OK || len(commands) != 2 || commands[1] != "umount -- /srv/pools/media" {
		t.Fatalf("unexpected pool unmount: %#v commands=%v", result, commands)
	}
	result = execute(request{Operation: "pool.unmount", OperationID: "pool-unmount", PlanHash: "pool-unmount", RequestedState: map[string]any{"mountPath": "/srv/pools/media"}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, nil, func(name string, _ ...string) ([]byte, error) {
		if name == "findmnt" {
			return []byte("ext4"), nil
		}
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "non-mergerfs") {
		t.Fatalf("expected non-mergerfs rejection: %#v", result)
	}
}

func TestNetworkCheckpointRejectsUnapprovedSetting(t *testing.T) {
	_, err := requestedChanges(map[string]any{"changes": map[string]any{"connection.secondaries": "bad"}})
	if err == nil || !strings.Contains(err.Error(), "not allow-listed") {
		t.Fatalf("expected network setting rejection, got %v", err)
	}
}

func TestTypedIdentityAndACLOperationsAreAllowListed(t *testing.T) {
	commands := make([]string, 0)
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	user := execute(request{Operation: "identity.system-user.ensure", OperationID: "identity-1", PlanHash: "identity", Confirmed: true, RequestedState: map[string]any{"name": "media", "uid": 1001, "gid": 1001, "home": "/srv/pools/media"}}, nil, run)
	if !user.OK || !strings.HasPrefix(commands[0], "useradd --system") {
		t.Fatalf("unexpected system-user result: %#v commands=%v", user, commands)
	}
	acl := execute(request{Operation: "acl.apply", OperationID: "acl-1", PlanHash: "acl", Confirmed: true, RequestedState: map[string]any{"path": "/srv/pools/media", "entries": []any{map[string]any{"principal": "media", "level": "read"}}}}, nil, run)
	if !acl.OK || !strings.HasPrefix(commands[1], "setfacl") {
		t.Fatalf("unexpected ACL result: %#v commands=%v", acl, commands)
	}
	unsafe := execute(request{Operation: "acl.apply", OperationID: "acl-1", PlanHash: "acl", Confirmed: true, RequestedState: map[string]any{"path": "/etc/passwd", "entries": []any{map[string]any{"principal": "media", "level": "write"}}}}, nil, run)
	if unsafe.OK || !strings.Contains(unsafe.Error, "allow-listed") {
		t.Fatalf("unsafe ACL path was accepted: %#v", unsafe)
	}
}

func TestFirewallApplyValidatesBeforeActivation(t *testing.T) {
	commands := make([]string, 0)
	result := execute(request{Operation: "firewall.apply", OperationID: "firewall-1", PlanHash: "firewall", Confirmed: true, RequestedState: map[string]any{"configPath": "/var/lib/lumonas/generated/nftables.conf"}}, nil, func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	})
	if !result.OK || len(commands) != 2 || commands[0] != "nft -c -f /var/lib/lumonas/generated/nftables.conf" || commands[1] != "nft -f /var/lib/lumonas/generated/nftables.conf" {
		t.Fatalf("unexpected firewall apply: %#v commands=%v", result, commands)
	}
}

func TestFirewallApplyRequiresOperationID(t *testing.T) {
	result := execute(request{Operation: "firewall.apply", PlanHash: "firewall", Confirmed: true, RequestedState: map[string]any{"configPath": "/var/lib/lumonas/generated/nftables.conf"}}, nil, func(string, ...string) ([]byte, error) {
		t.Fatal("firewall command ran without an operation ID")
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "operationId is required") {
		t.Fatalf("firewall mutation without an operation ID was accepted: %#v", result)
	}
}

func TestShareActivationRequiresOperationID(t *testing.T) {
	for _, operation := range []string{"service.config.apply", "avahi.config.apply", "identity.system-user.ensure", "samba.user.ensure", "acl.apply"} {
		t.Run(operation, func(t *testing.T) {
			result := execute(request{Operation: operation, PlanHash: "share-plan", Confirmed: true}, nil, func(string, ...string) ([]byte, error) {
				t.Fatal("share activation command ran without an operation ID")
				return nil, nil
			})
			if result.OK || !strings.Contains(result.Error, "operationId is required") {
				t.Fatalf("share mutation without an operation ID was accepted: %#v", result)
			}
		})
	}
}

func TestSambaClientDisconnectUsesValidatedIP(t *testing.T) {
	called := false
	invalid := execute(request{Operation: "samba.client.disconnect", OperationID: "disconnect-1", PlanHash: "disconnect", Confirmed: true, RequestedState: map[string]any{"address": "; reboot"}}, nil, func(string, ...string) ([]byte, error) {
		called = true
		return nil, nil
	})
	if invalid.OK || called {
		t.Fatalf("invalid client IP reached smbcontrol: %#v", invalid)
	}
	valid := execute(request{Operation: "samba.client.disconnect", OperationID: "disconnect-2", PlanHash: "disconnect", Confirmed: true, RequestedState: map[string]any{"address": "192.0.2.41"}}, nil, func(name string, args ...string) ([]byte, error) {
		called = true
		if name != "smbcontrol" || strings.Join(args, " ") != "smbd kill-client-ip 192.0.2.41" {
			t.Fatalf("unexpected disconnect command: %s %v", name, args)
		}
		return []byte("ok"), nil
	})
	if !valid.OK || !called {
		t.Fatalf("valid confirmed disconnect failed: %#v", valid)
	}
}

func TestRemainingMutationsRequireOperationID(t *testing.T) {
	for _, operation := range []string{
		"network.checkpoint.begin", "network.checkpoint.commit", "network.checkpoint.rollback",
		"network.wifi.connect", "network.wol.set", "service.reload", "power.action", "power.shutdown",
		"samba.client.disconnect", "tls.certificate.install",
	} {
		t.Run(operation, func(t *testing.T) {
			result := execute(request{Operation: operation, PlanHash: "mutation-plan", Confirmed: true}, nil, func(string, ...string) ([]byte, error) {
				t.Fatal("mutation command ran without an operation ID")
				return nil, nil
			})
			if result.OK || !strings.Contains(result.Error, "operationId is required") {
				t.Fatalf("mutation without an operation ID was accepted: %#v", result)
			}
		})
	}
}

func TestSambaUserProvisioningUsesStdinPasswords(t *testing.T) {
	commands := make([]string, 0)
	stdinCommands := make([]string, 0)
	stdinPayload := ""
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	stdinRun := func(name string, args []string, stdin string) ([]byte, error) {
		stdinCommands = append(stdinCommands, name+" "+strings.Join(args, " "))
		stdinPayload = stdin
		return nil, nil
	}
	base := request{Operation: "samba.user.ensure", OperationID: "samba-1", PlanHash: "samba", Confirmed: true}
	created := ensureSambaUser(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, RequestedState: map[string]any{"name": "media", "create": true, "password": "a-file-user-password"}}, run, stdinRun)
	if !created.OK || len(stdinCommands) != 1 || stdinCommands[0] != "pdbedit -a -t -u media" || stdinPayload != "a-file-user-password\na-file-user-password\n" {
		t.Fatalf("unexpected create result %#v stdin=%v payload=%q", created, stdinCommands, stdinPayload)
	}
	rotated := ensureSambaUser(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, RequestedState: map[string]any{"name": "media", "password": "rotated-password-1"}}, run, stdinRun)
	if !rotated.OK || len(stdinCommands) != 2 || stdinCommands[1] != "pdbedit -t -u media" || stdinPayload != "rotated-password-1\nrotated-password-1\n" {
		t.Fatalf("unexpected rotation result %#v stdin=%v", rotated, stdinCommands)
	}
	disabled := ensureSambaUser(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, RequestedState: map[string]any{"name": "media", "disabled": true}}, run, stdinRun)
	if !disabled.OK || len(commands) != 1 || commands[0] != "pdbedit -u media -d" {
		t.Fatalf("unexpected disable result %#v commands=%v", disabled, commands)
	}
	enabled := ensureSambaUser(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, RequestedState: map[string]any{"name": "media", "enable": true}}, run, stdinRun)
	if !enabled.OK || len(commands) != 2 || commands[1] != "pdbedit -u media -e" {
		t.Fatalf("unexpected enable result %#v commands=%v", enabled, commands)
	}
}

func TestSambaUserProvisioningRejectsUnsafeInput(t *testing.T) {
	base := request{Operation: "samba.user.ensure", OperationID: "samba-1", PlanHash: "samba", Confirmed: true}
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	stdinRun := func(string, []string, string) ([]byte, error) { return nil, nil }
	cases := []struct {
		name     string
		state    map[string]any
		expected string
	}{
		{"short password", map[string]any{"name": "media", "create": true, "password": "short"}, "invalid"},
		{"password with newline", map[string]any{"name": "media", "create": true, "password": "one\ntwo"}, "invalid"},
		{"password and disabled", map[string]any{"name": "media", "disabled": true, "password": "a-file-user-password"}, "mutually exclusive"},
		{"no actionable flag", map[string]any{"name": "media"}, "requires password"},
	}
	for _, testCase := range cases {
		result := ensureSambaUser(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, RequestedState: testCase.state}, run, stdinRun)
		if result.OK || !strings.Contains(result.Error, testCase.expected) {
			t.Fatalf("%s: expected %q rejection, got %#v", testCase.name, testCase.expected, result)
		}
	}
}

func TestSystemUserLockAndUnlockAreAllowListed(t *testing.T) {
	commands := make([]string, 0)
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	base := request{Operation: "identity.system-user.ensure", OperationID: "identity-1", PlanHash: "identity", Confirmed: true}
	locked := ensureSystemUser(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, RequestedState: map[string]any{"name": "media", "disabled": true}}, run)
	if !locked.OK || commands[len(commands)-1] != "usermod -L media" {
		t.Fatalf("unexpected lock result %#v commands=%v", locked, commands)
	}
	unlocked := ensureSystemUser(request{Operation: base.Operation, PlanHash: base.PlanHash, Confirmed: true, RequestedState: map[string]any{"name": "media", "enable": true}}, run)
	if !unlocked.OK || commands[len(commands)-1] != "usermod -U media" {
		t.Fatalf("unexpected unlock result %#v commands=%v", unlocked, commands)
	}
}
