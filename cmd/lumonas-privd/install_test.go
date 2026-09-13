package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/model"
)

func TestInstallApplyKeepsCredentialOutOfArguments(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "install-disk")
	argsPath := filepath.Join(root, "args")
	stdinPath := filepath.Join(root, "stdin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$*\" >\"$LUMONAS_TEST_INSTALL_ARGS\"\ncat >\"$LUMONAS_TEST_INSTALL_STDIN\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_INSTALL_SCRIPT", script)
	t.Setenv("LUMONAS_TEST_INSTALL_ARGS", argsPath)
	t.Setenv("LUMONAS_TEST_INSTALL_STDIN", stdinPath)

	request := request{
		Operation:    "install.apply",
		OperationID:  "install-1",
		PlanHash:     "plan-1",
		TargetDiskID: "wwn:target",
		Confirmed:    true,
		RequestedState: map[string]any{
			"filesystem":    "ext4",
			"uefi":          true,
			"hostname":      "lumonas",
			"adminUsername": "admin",
			"adminPassword": "a-very-strong-test-password",
		},
	}
	disk := model.Disk{ID: "wwn:target", CurrentPath: "/dev/vdb", SizeBytes: 16 << 30, WWN: "wwn:target"}
	result := applyDiskInstall(request, disk, nil, nil)
	if !result.OK {
		t.Fatalf("install failed: %#v", result)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "a-very-strong-test-password") {
		t.Fatalf("password hash leaked into installer arguments: %q", args)
	}
	stdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != "a-very-strong-test-password\n" {
		t.Fatalf("installer stdin = %q", stdin)
	}
}

func TestExecuteInstallApplyFailsClosedOnMountDiscovery(t *testing.T) {
	disk := model.Disk{ID: "wwn:target", CurrentPath: "/dev/vdb", SizeBytes: 16 << 30, WWN: "wwn:target"}
	req := request{Operation: "install.apply", OperationID: "install-2", PlanHash: "plan-2", TargetDiskID: disk.ID, Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute), ExpectedIdentity: map[string]string{"wwn": disk.WWN}, RequestedState: map[string]any{"filesystem": "ext4", "hostname": "lumonas", "adminUsername": "admin", "adminPassword": "a-very-strong-test-password"}}
	result := execute(req, func(_ collector.CommandRunner) ([]model.Disk, error) { return []model.Disk{disk}, nil }, func(name string, args ...string) ([]byte, error) {
		if name == "findmnt" {
			return nil, os.ErrNotExist
		}
		return nil, os.ErrNotExist
	})
	if result.OK || !strings.Contains(result.Error, "could not verify target mount state") {
		t.Fatalf("expected fail-closed mount error, got %#v", result)
	}
}
