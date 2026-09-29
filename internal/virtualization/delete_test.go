package virtualization

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func managedVMFixture(t *testing.T) (string, string, string) {
	t.Helper()
	directory := t.TempDir()
	definitionPath := filepath.Join(directory, "guest-1.xml")
	diskPath := filepath.Join(directory, "guest-1.qcow2")
	isoPath := filepath.Join(directory, "installer.iso")
	isoBytes := make([]byte, 32_768+7)
	copy(isoBytes[32_768:], []byte{1, 'C', 'D', '0', '0', '1', 1})
	if err := os.WriteFile(isoPath, isoBytes, 0600); err != nil {
		t.Fatal(err)
	}
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + diskPath + `"/><target dev="vda"/></disk><disk type="file" device="cdrom"><source file="` + isoPath + `"/></disk></devices></domain>`
	if err := os.WriteFile(definitionPath, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskPath, []byte("qcow2-data"), 0600); err != nil {
		t.Fatal(err)
	}
	return directory, definitionPath, diskPath
}

func TestDeleteManagedVMAndOptionallyKeepDisk(t *testing.T) {
	for _, deleteDisk := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep disk", true: "delete disk"}[deleteDisk], func(t *testing.T) {
			directory, definitionPath, diskPath := managedVMFixture(t)
			var commands []string
			service := New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
				commands = append(commands, strings.Join(args, " "))
				switch strings.Join(args, " ") {
				case "domstate guest-1":
					return []byte("shut off\n"), nil
				case "undefine guest-1 --managed-save --snapshots-metadata":
					return nil, nil
				default:
					t.Fatalf("unexpected command: %v", args)
					return nil, nil
				}
			})
			service.VMDir = directory
			service.ISODir = directory
			result, err := service.Delete(context.Background(), "guest-1", deleteDisk)
			if err != nil {
				t.Fatal(err)
			}
			if result.Name != "guest-1" || result.DiskPath != diskPath || result.DiskRemoved != deleteDisk {
				t.Fatalf("unexpected delete result: %#v", result)
			}
			_, definitionErr := os.Lstat(definitionPath)
			if deleteDisk && !os.IsNotExist(definitionErr) {
				t.Fatalf("managed definition remains: %v", definitionErr)
			}
			if !deleteDisk && definitionErr != nil {
				t.Fatalf("saved definition should have been retained: %v", definitionErr)
			}
			_, diskErr := os.Lstat(diskPath)
			if deleteDisk && !os.IsNotExist(diskErr) {
				t.Fatalf("disk was not removed: %v", diskErr)
			}
			if !deleteDisk && diskErr != nil {
				t.Fatalf("disk should have been kept: %v", diskErr)
			}
			if len(commands) != 2 {
				t.Fatalf("unexpected command sequence: %v", commands)
			}
		})
	}
}

func TestDeleteVMRequiresStoppedManagedGuest(t *testing.T) {
	directory, definitionPath, diskPath := managedVMFixture(t)
	commands := 0
	service := New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		commands++
		if strings.Join(args, " ") != "domstate guest-1" {
			t.Fatalf("unexpected command after unsafe state: %v", args)
		}
		return []byte("running\n"), nil
	})
	service.VMDir = directory
	service.ISODir = directory
	if _, err := service.Delete(context.Background(), "guest-1", true); err == nil || !strings.Contains(err.Error(), "shut down") {
		t.Fatalf("running VM delete error = %v", err)
	}
	if commands != 1 {
		t.Fatalf("running VM proceeded to undefine: commands=%d", commands)
	}

	if err := os.Remove(definitionPath); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Delete(context.Background(), "guest-1", true); err == nil || !strings.Contains(err.Error(), "not a LumoNAS-managed") {
		t.Fatalf("unmanaged VM delete error = %v", err)
	}
	if _, err := os.Stat(diskPath); err != nil {
		t.Fatalf("unmanaged disk was touched: %v", err)
	}
}

func TestRecoverableVMDefinitionVerifiesDiskBeforeRestore(t *testing.T) {
	directory, _, diskPath := managedVMFixture(t)
	defined := false
	var commands []string
	service := New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		commands = append(commands, binary+" "+joined)
		switch joined {
		case "list --all --name":
			if defined {
				return []byte("guest-1\n"), nil
			}
			return nil, nil
		case "check -f qcow2 " + diskPath:
			return nil, nil
		case "define --validate " + filepath.Join(directory, "guest-1.xml"):
			defined = true
			return nil, nil
		case "dominfo guest-1":
			return []byte("Name: guest-1\nState: shut off\nCPU(s): 2\n"), nil
		default:
			t.Fatalf("unexpected command: %s", joined)
			return nil, nil
		}
	})
	service.VMDir = directory
	service.ISODir = directory

	recoverables, err := service.RecoverableDefinitions(context.Background())
	if err != nil || len(recoverables) != 1 || recoverables[0].Name != "guest-1" || recoverables[0].DiskBytes == 0 {
		t.Fatalf("recoverable definitions = %#v, err=%v", recoverables, err)
	}
	domain, err := service.RestoreDefinition(context.Background(), "guest-1")
	if err != nil || domain.Name != "guest-1" || domain.State != "shut off" {
		t.Fatalf("restored VM domain = %#v, err=%v", domain, err)
	}
	if !defined || !strings.Contains(strings.Join(commands, "\n"), "qemu-img check -f qcow2 "+diskPath) {
		t.Fatalf("restore did not verify and define the guest: %v", commands)
	}
}
