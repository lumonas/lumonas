package virtualization

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requireLibvirtGroup skips tests that stage qemu backups, because the staging
// helper chowns to the libvirt group. The packaged appliance depends on that
// package, but a bare CI or developer machine may not have it.
func requireLibvirtGroup(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := user.LookupGroup("libvirt"); err != nil {
		t.Skip("libvirt group is not available in this test environment")
	}
}

func TestBackupManagedDisksConvertsStoppedManagedVM(t *testing.T) {
	requireLibvirtGroup(t)
	vmDir := t.TempDir()
	recoveryDir := t.TempDir()
	definitionPath := filepath.Join(vmDir, "guest-1.xml")
	diskPath := filepath.Join(vmDir, "guest-1.qcow2")
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + diskPath + `"/><target dev="vda"/></disk></devices></domain>`
	if err := os.WriteFile(definitionPath, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskPath, []byte("source qcow2"), 0600); err != nil {
		t.Fatal(err)
	}
	var commands []string
	service := New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, binary+" "+command)
		switch command {
		case "list --all --name":
			return []byte("guest-1\n"), nil
		case "dominfo guest-1":
			return []byte("Name: guest-1\nState: shut off\n"), nil
		default:
			if binary == "qemu-img" && strings.HasPrefix(command, "convert ") {
				if err := os.WriteFile(strings.Fields(command)[len(strings.Fields(command))-1], []byte("verified snapshot"), 0600); err != nil {
					return nil, err
				}
				return nil, nil
			}
			if binary == "qemu-img" && strings.HasPrefix(command, "check ") {
				return nil, nil
			}
			t.Fatalf("unexpected command: %s %s", binary, command)
			return nil, nil
		}
	})
	service.VMDir = vmDir
	artifacts, err := service.BackupManagedDisks(context.Background(), recoveryDir, "run-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "guest-1" || len(artifacts[0].Disks) != 1 || artifacts[0].Disks[0].Bytes == 0 || artifacts[0].Definition.Bytes == 0 || artifacts[0].Disks[0].Checksum == "" || artifacts[0].Definition.Checksum == "" {
		t.Fatalf("unexpected backup artifacts: %#v", artifacts)
	}
	if got, err := os.ReadFile(artifacts[0].Disks[0].Path); err != nil || string(got) != "verified snapshot" {
		t.Fatalf("published disk = %q, error = %v", got, err)
	}
	if got, err := os.ReadFile(artifacts[0].Definition.Path); err != nil || string(got) != definition {
		t.Fatalf("published definition = %q, error = %v", got, err)
	}
	if len(commands) != 6 || !strings.Contains(strings.Join(commands, "\n"), "qemu-img convert") || !strings.Contains(strings.Join(commands, "\n"), "qemu-img check") {
		t.Fatalf("unexpected command sequence: %v", commands)
	}
}

func TestBackupManagedDisksFailsClosedForUnmanagedVM(t *testing.T) {
	requireLibvirtGroup(t)
	vmDir := t.TempDir()
	service := New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "list --all --name":
			return []byte("external-vm\n"), nil
		case "dominfo external-vm":
			return []byte("Name: external-vm\nState: shut off\n"), nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return nil, nil
		}
	})
	service.VMDir = vmDir
	if _, err := service.BackupManagedDisks(context.Background(), t.TempDir(), "run-456"); err == nil || !strings.Contains(err.Error(), "not managed by LumoNAS") {
		t.Fatalf("unmanaged VM backup error = %v", err)
	}
}

func TestBackupManagedDisksIncludesEveryDiskAndInstallerISO(t *testing.T) {
	requireLibvirtGroup(t)
	vmDir := t.TempDir()
	mediaDir := t.TempDir()
	recoveryDir := t.TempDir()
	diskA := filepath.Join(vmDir, "guest-1.qcow2")
	diskB := filepath.Join(vmDir, "guest-1-data.qcow2")
	iso := filepath.Join(mediaDir, "installer.iso")
	for path, contents := range map[string]string{diskA: "system disk", diskB: "data disk"} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	isoBytes := make([]byte, 32_768+7)
	copy(isoBytes[32_768:], []byte{1, 'C', 'D', '0', '0', '1', 1})
	if err := os.WriteFile(iso, isoBytes, 0600); err != nil {
		t.Fatal(err)
	}
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + diskA + `"/><target dev="vda"/></disk><disk type="file" device="disk"><source file="` + diskB + `"/><target dev="vdb"/></disk><disk type="file" device="cdrom"><source file="` + iso + `"/></disk></devices></domain>`
	if err := os.WriteFile(filepath.Join(vmDir, "guest-1.xml"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	commands := 0
	service := New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		commands++
		command := strings.Join(args, " ")
		switch command {
		case "list --all --name":
			return []byte("guest-1\n"), nil
		case "dominfo guest-1":
			return []byte("Name: guest-1\nState: shut off\n"), nil
		default:
			if binary == "qemu-img" && strings.HasPrefix(command, "convert ") {
				if err := os.WriteFile(args[len(args)-1], []byte("converted:"+filepath.Base(args[len(args)-2])), 0600); err != nil {
					return nil, err
				}
				return nil, nil
			}
			if binary == "qemu-img" && strings.HasPrefix(command, "check ") {
				return nil, nil
			}
			t.Fatalf("unexpected command: %s %s", binary, command)
			return nil, nil
		}
	})
	service.VMDir, service.ISODir = vmDir, mediaDir
	artifacts, err := service.BackupManagedDisks(context.Background(), recoveryDir, "run-multi")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || len(artifacts[0].Disks) != 2 || len(artifacts[0].Media) != 1 {
		t.Fatalf("backup omitted a VM disk or installer ISO: %#v", artifacts)
	}
	if commands != 10 {
		t.Fatalf("unexpected VM backup command count %d", commands)
	}
	if err := service.ValidateBackupMedia("installer.iso", artifacts[0].Media[0].Path); err != nil {
		t.Fatalf("published installer ISO is invalid: %v", err)
	}
}

func TestRunningMultiDiskBackupUsesOneConsistentLibvirtJob(t *testing.T) {
	requireLibvirtGroup(t)
	vmDir := t.TempDir()
	recoveryDir := t.TempDir()
	diskA := filepath.Join(vmDir, "guest-1.qcow2")
	diskB := filepath.Join(vmDir, "guest-1-data.qcow2")
	for _, path := range []string{diskA, diskB} {
		if err := os.WriteFile(path, []byte("current disk data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + diskA + `"/><target dev="vda"/></disk><disk type="file" device="disk"><source file="` + diskB + `"/><target dev="vdb"/></disk></devices></domain>`
	if err := os.WriteFile(filepath.Join(vmDir, "guest-1.xml"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	var liveBackupJobs int
	var backupXMLTargets []string
	service := New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		switch command {
		case "list --all --name":
			return []byte("guest-1\n"), nil
		case "dominfo guest-1":
			return []byte("Name: guest-1\nState: running\n"), nil
		case "domblkinfo guest-1 vda", "domblkinfo guest-1 vdb":
			return []byte("Capacity: 1048576\nAllocation: 4096\nPhysical: 4096\n"), nil
		case "domjobinfo guest-1":
			return []byte("Job type: None\n"), nil
		case "domjobinfo guest-1 --completed --keep-completed":
			return []byte("Operation: Backup\nJob type: Completed\n"), nil
		default:
			if binary == "virsh" && len(args) == 4 && args[0] == "backup-begin" {
				liveBackupJobs++
				data, err := os.ReadFile(args[2])
				if err != nil {
					return nil, err
				}
				var backupXML domainBackupXML
				if err := xml.Unmarshal(data, &backupXML); err != nil {
					return nil, err
				}
				for _, disk := range backupXML.Disks {
					backupXMLTargets = append(backupXMLTargets, disk.Name)
				}
				if len(backupXML.Disks) != 2 {
					return nil, errors.New("live backup XML did not contain both disks")
				}
				return nil, nil
			}
			if binary == "qemu-img" && len(args) >= 1 && args[0] == "create" {
				if err := os.WriteFile(args[len(args)-2], []byte("created qcow2"), 0600); err != nil {
					return nil, err
				}
				return nil, nil
			}
			if binary == "qemu-img" && len(args) >= 1 && args[0] == "convert" {
				if err := os.WriteFile(args[len(args)-1], []byte("converted qcow2"), 0600); err != nil {
					return nil, err
				}
				return nil, nil
			}
			if binary == "qemu-img" && len(args) >= 1 && args[0] == "check" {
				return nil, nil
			}
			t.Fatalf("unexpected command: %s %s", binary, command)
			return nil, nil
		}
	})
	service.VMDir = vmDir
	artifacts, err := service.BackupManagedDisks(context.Background(), recoveryDir, "run-live")
	if err != nil {
		t.Fatal(err)
	}
	if liveBackupJobs != 1 || len(backupXMLTargets) != 2 || len(artifacts) != 1 || len(artifacts[0].Disks) != 2 {
		t.Fatalf("running multi-disk backup was not a single consistent job: jobs=%d targets=%v artifacts=%#v", liveBackupJobs, backupXMLTargets, artifacts)
	}
}

func TestBackupManagedDisksDoesNotRequireLibvirtWithoutManagedVMs(t *testing.T) {
	service := New(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, os.ErrPermission
	})
	service.VMDir = filepath.Join(t.TempDir(), "missing")
	artifacts, err := service.BackupManagedDisks(context.Background(), t.TempDir(), "run-789")
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("backup without managed VMs = %#v, error = %v", artifacts, err)
	}
}

func TestRestoreBackupGuestInstallsValidatedFilesWithoutStartingVM(t *testing.T) {
	requireLibvirtGroup(t)
	vmDir := t.TempDir()
	backupDir := t.TempDir()
	diskSource := filepath.Join(backupDir, "guest-1.qcow2")
	definitionSource := filepath.Join(backupDir, "guest-1.xml")
	if err := os.WriteFile(diskSource, []byte("verified qcow2"), 0600); err != nil {
		t.Fatal(err)
	}
	diskTarget := filepath.Join(vmDir, "guest-1.qcow2")
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + diskTarget + `"/><target dev="vda"/></disk></devices></domain>`
	if err := os.WriteFile(definitionSource, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	var commands []string
	service := New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, binary+" "+command)
		switch command {
		case "list --all --name":
			return nil, nil
		case "define --validate " + filepath.Join(vmDir, "guest-1.xml"):
			return nil, nil
		case "dominfo guest-1":
			return []byte("Name: guest-1\nState: shut off\n"), nil
		default:
			if binary == "qemu-img" && strings.HasPrefix(command, "convert ") {
				if err := os.WriteFile(args[len(args)-1], []byte("verified qcow2"), 0600); err != nil {
					return nil, err
				}
				return nil, nil
			}
			if binary == "qemu-img" && strings.HasPrefix(command, "check ") {
				return nil, nil
			}
			t.Fatalf("unexpected command: %s %s", binary, command)
			return nil, nil
		}
	})
	service.VMDir = vmDir
	domain, err := service.RestoreBackupGuest(context.Background(), "guest-1", definitionSource, map[string]string{"guest-1.qcow2": diskSource}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if domain.Name != "guest-1" || domain.State != "shut off" {
		t.Fatalf("unexpected restored domain: %#v", domain)
	}
	if len(commands) != 6 || strings.Contains(strings.Join(commands, "\n"), " start ") {
		t.Fatalf("restore should validate and define without starting the guest: %v", commands)
	}
	if data, err := os.ReadFile(diskTarget); err != nil || string(data) != "verified qcow2" {
		t.Fatalf("restored disk = %q, error = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(vmDir, "guest-1.xml")); err != nil {
		t.Fatalf("restored definition missing: %v", err)
	}
}

func TestRestoreBackupGuestCleansFilesWhenLibvirtRejectsDefinition(t *testing.T) {
	requireLibvirtGroup(t)
	vmDir := t.TempDir()
	backupDir := t.TempDir()
	diskSource := filepath.Join(backupDir, "guest-1.qcow2")
	definitionSource := filepath.Join(backupDir, "guest-1.xml")
	if err := os.WriteFile(diskSource, []byte("verified qcow2"), 0600); err != nil {
		t.Fatal(err)
	}
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + filepath.Join(vmDir, "guest-1.qcow2") + `"/><target dev="vda"/></disk></devices></domain>`
	if err := os.WriteFile(definitionSource, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	service := New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		switch command {
		case "list --all --name":
			return nil, nil
		case "define --validate " + filepath.Join(vmDir, "guest-1.xml"):
			return nil, os.ErrPermission
		default:
			if strings.HasPrefix(command, "convert ") {
				if err := os.WriteFile(args[len(args)-1], []byte("verified qcow2"), 0600); err != nil {
					return nil, err
				}
			}
			return nil, nil
		}
	})
	service.VMDir = vmDir
	if _, err := service.RestoreBackupGuest(context.Background(), "guest-1", definitionSource, map[string]string{"guest-1.qcow2": diskSource}, nil); err == nil {
		t.Fatal("expected libvirt definition error")
	}
	for _, name := range []string{"guest-1.qcow2", "guest-1.xml"} {
		if _, err := os.Lstat(filepath.Join(vmDir, name)); !os.IsNotExist(err) {
			t.Fatalf("failed restore left %s behind: %v", name, err)
		}
	}
}

func TestRestoreBackupGuestRestoresMultipleDisksAndInstallerMedia(t *testing.T) {
	requireLibvirtGroup(t)
	vmDir := t.TempDir()
	mediaDir := filepath.Join(t.TempDir(), "media")
	backupDir := t.TempDir()
	diskA := filepath.Join(backupDir, "guest-system.qcow2")
	diskB := filepath.Join(backupDir, "guest-data.qcow2")
	definitionSource := filepath.Join(backupDir, "definition.xml")
	isoSource := filepath.Join(backupDir, "installer.iso")
	for path, contents := range map[string]string{diskA: "system image", diskB: "data image"} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	isoBytes := make([]byte, 32_768+7)
	copy(isoBytes[32_768:], []byte{1, 'C', 'D', '0', '0', '1', 1})
	if err := os.WriteFile(isoSource, isoBytes, 0600); err != nil {
		t.Fatal(err)
	}
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + filepath.Join(vmDir, "system.qcow2") + `"/><target dev="vda"/></disk><disk type="file" device="disk"><source file="` + filepath.Join(vmDir, "data.qcow2") + `"/><target dev="vdb"/></disk><disk type="file" device="cdrom"><source file="` + filepath.Join(mediaDir, "installer.iso") + `"/></disk></devices></domain>`
	if err := os.WriteFile(definitionSource, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	service := New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		switch command {
		case "list --all --name":
			return nil, nil
		case "define --validate " + filepath.Join(vmDir, "guest-1.xml"):
			return nil, nil
		case "dominfo guest-1":
			return []byte("Name: guest-1\nState: shut off\n"), nil
		default:
			if binary == "qemu-img" && strings.HasPrefix(command, "convert ") {
				input, err := os.ReadFile(args[len(args)-2])
				if err != nil {
					return nil, err
				}
				if err := os.WriteFile(args[len(args)-1], input, 0600); err != nil {
					return nil, err
				}
				return nil, nil
			}
			if binary == "qemu-img" && strings.HasPrefix(command, "check ") {
				return nil, nil
			}
			t.Fatalf("unexpected command: %s %s", binary, command)
			return nil, nil
		}
	})
	service.VMDir, service.ISODir = vmDir, mediaDir
	_, err := service.RestoreBackupGuest(context.Background(), "guest-1", definitionSource, map[string]string{"system.qcow2": diskA, "data.qcow2": diskB}, map[string]string{"installer.iso": isoSource})
	if err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string]string{"system.qcow2": "system image", "data.qcow2": "data image"} {
		data, err := os.ReadFile(filepath.Join(vmDir, name))
		if err != nil || string(data) != expected {
			t.Fatalf("restored disk %s = %q, error = %v", name, data, err)
		}
	}
	if err := service.ValidateBackupMedia("installer.iso", filepath.Join(mediaDir, "installer.iso")); err != nil {
		t.Fatalf("restored installer media invalid: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(vmDir, "guest-1.xml")); err != nil || string(data) != definition {
		t.Fatalf("restored definition = %q, error = %v", data, err)
	}
}
