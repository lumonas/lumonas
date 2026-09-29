package virtualization

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDomainsListsAndParsesLibvirtDefinitions(t *testing.T) {
	service := New(func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "virsh" {
			t.Fatalf("unexpected executable %q", name)
		}
		switch strings.Join(args, " ") {
		case "list --all --name":
			return []byte("media-vm\n\n"), nil
		case "dominfo media-vm":
			return []byte("Name: media-vm\nUUID: 11111111-1111-1111-1111-111111111111\nState: running\nCPU(s): 4\nMax memory: 2097152 KiB\nUsed memory: 1024000 KiB\n"), nil
		default:
			t.Fatalf("unexpected virsh args %v", args)
			return nil, nil
		}
	})
	domains, err := service.Domains(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Domain{{Name: "media-vm", UUID: "11111111-1111-1111-1111-111111111111", State: "running", VCPUs: 4, MemoryKiB: 1024000, MaximumMemory: 2097152}}
	if !reflect.DeepEqual(domains, want) {
		t.Fatalf("domains = %#v, want %#v", domains, want)
	}
}

func TestActionUsesFixedVerbsAndRejectsUnsupportedOrUnsafeInput(t *testing.T) {
	var commands [][]string
	service := New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		if reflect.DeepEqual(args, []string{"dominfo", "guest-1"}) {
			return []byte("State: shut off\n"), nil
		}
		return nil, nil
	})
	domain, err := service.Action(context.Background(), "guest-1", "shutdown")
	if err != nil || domain.State != "shut off" || !reflect.DeepEqual(commands, [][]string{{"shutdown", "guest-1"}, {"dominfo", "guest-1"}}) {
		t.Fatalf("unexpected action result %#v commands=%v err=%v", domain, commands, err)
	}
	if _, err := service.Action(context.Background(), "guest-1;touch /tmp/pwn", "start"); err == nil {
		t.Fatal("unsafe domain name was accepted")
	}
	if _, err := service.Action(context.Background(), "guest-1", "destroy"); err == nil {
		t.Fatal("unsupported action was accepted")
	}
}

type testConsoleRuntime struct {
	started  int
	closed   int
	input    []byte
	state    ConsoleState
	startErr error
}

func (runtime *testConsoleRuntime) Start(_ context.Context, virsh, domain string) error {
	if virsh != "virsh" || domain != "guest-1" {
		return errors.New("unexpected console target")
	}
	runtime.started++
	return runtime.startErr
}
func (runtime *testConsoleRuntime) Read(_ string, cursor int64) (ConsoleState, error) {
	return runtime.state, nil
}
func (runtime *testConsoleRuntime) Write(_ string, data []byte) error {
	runtime.input = append(runtime.input, data...)
	return nil
}
func (runtime *testConsoleRuntime) Close(string) error { runtime.closed++; return nil }

func TestSerialConsoleRequiresRunningGuestAndBoundsInput(t *testing.T) {
	console := &testConsoleRuntime{state: ConsoleState{Output: "installer ready", Cursor: 15, Connected: true}}
	service := New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") != "domstate guest-1" {
			t.Fatalf("unexpected command %v", args)
		}
		return []byte("running\n"), nil
	})
	service.ConsoleRuntime = console
	if err := service.OpenConsole(context.Background(), "guest-1"); err != nil || console.started != 1 {
		t.Fatalf("open serial console: starts=%d err=%v", console.started, err)
	}
	state, err := service.ReadConsole("guest-1", 0)
	if err != nil || state.Output != "installer ready" || !state.Connected {
		t.Fatalf("read serial console: %#v err=%v", state, err)
	}
	if err := service.WriteConsole("guest-1", []byte("\x1b[A")); err != nil || string(console.input) != "\x1b[A" {
		t.Fatalf("write serial input %q err=%v", console.input, err)
	}
	if err := service.WriteConsole("guest-1", make([]byte, MaxConsoleInputBytes+1)); err == nil {
		t.Fatal("oversized console input was accepted")
	}
	if err := service.OpenConsole(context.Background(), "guest-1;rm"); err == nil {
		t.Fatal("unsafe VM name was accepted for console")
	}
	if err := service.CloseConsole("guest-1"); err != nil || console.closed != 1 {
		t.Fatalf("close serial console: closed=%d err=%v", console.closed, err)
	}

	service.Run = func(context.Context, string, ...string) ([]byte, error) { return []byte("shut off\n"), nil }
	console.started = 0
	if err := service.OpenConsole(context.Background(), "guest-1"); err == nil || console.started != 0 {
		t.Fatal("console opened for a stopped guest")
	}
}

func TestCreateUsesUploadedISOAndBoundedLibvirtCommands(t *testing.T) {
	root := t.TempDir()
	isoDir, vmDir := filepath.Join(root, "isos"), filepath.Join(root, "vms")
	if err := os.MkdirAll(isoDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(isoDir, "ubuntu.iso"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	memInfo := filepath.Join(root, "meminfo")
	if err := os.WriteFile(memInfo, []byte("MemTotal:       8388608 kB\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var commands [][]string
	service := New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{binary}, args...))
		joined := strings.Join(args, " ")
		switch {
		case binary == "qemu-img" && len(args) == 5 && args[0] == "create":
			return nil, os.WriteFile(args[3], []byte("qcow2 fixture"), 0600)
		case joined == "uri":
			return []byte("qemu:///system"), nil
		case joined == "list --all --name":
			return nil, nil
		case strings.HasPrefix(joined, "define --validate "):
			return nil, nil
		case joined == "start e2e-guest":
			return nil, nil
		case joined == "dominfo e2e-guest":
			return []byte("Name: e2e-guest\nState: running\nCPU(s): 2\nMax memory: 2097152 KiB\n"), nil
		default:
			t.Fatalf("unexpected command: %s %v", binary, args)
			return nil, nil
		}
	})
	service.ISODir, service.VMDir, service.MemInfo = isoDir, vmDir, memInfo
	created, err := service.Create(context.Background(), CreateInput{Name: "e2e-guest", ISO: "ubuntu.iso", VCPUs: 2, MemoryMiB: 2048, DiskGiB: 32})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "e2e-guest" || created.State != "running" || created.ISO != "ubuntu.iso" {
		t.Fatalf("unexpected created domain: %#v", created)
	}
	if _, err := os.Stat(created.DiskPath); err != nil {
		t.Fatalf("VM disk was not created: %v", err)
	}
	xmlPath := filepath.Join(vmDir, "e2e-guest.xml")
	definition, err := os.ReadFile(xmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(definition), `<domain type="qemu">`) && !strings.Contains(string(definition), `<domain type="kvm">`) {
		t.Fatalf("VM definition has no supported hypervisor type: %s", definition)
	}
	for _, expected := range []string{"<name>e2e-guest</name>", "<memory unit=\"MiB\">2048</memory>", `<boot dev="cdrom"></boot>`, "ubuntu.iso", "127.0.0.1"} {
		if !strings.Contains(string(definition), expected) {
			t.Fatalf("VM definition is missing %q: %s", expected, definition)
		}
	}
	if len(commands) != 6 || commands[2][0] != "qemu-img" || strings.Join(commands[4][1:], " ") != "start e2e-guest" {
		t.Fatalf("unexpected command sequence: %v", commands)
	}
}

func TestStoreISORejectsUnsafeAndDuplicateNames(t *testing.T) {
	service := New(func(context.Context, string, ...string) ([]byte, error) { return nil, nil })
	service.ISODir = t.TempDir()
	if _, err := service.StoreISO("../unsafe.iso", strings.NewReader("iso")); err == nil {
		t.Fatal("path traversal ISO name was accepted")
	}
	if _, err := service.StoreISO("windows.exe", strings.NewReader("iso")); err == nil {
		t.Fatal("non-ISO extension was accepted")
	}
	media, err := service.StoreISO("fixture.iso", strings.NewReader("iso"))
	if err == nil {
		t.Fatal("non-ISO image was accepted")
	}
	media, err = service.StoreISO("fixture.iso", strings.NewReader(string(testISOImage())))
	if err != nil || media.SizeBytes != int64(len(testISOImage())) {
		t.Fatalf("store ISO = %#v, %v", media, err)
	}
	if _, err := service.StoreISO("fixture.iso", strings.NewReader("again")); err == nil {
		t.Fatal("duplicate ISO filename was overwritten")
	}
}

func TestSnapshotsUseValidatedLibvirtOperations(t *testing.T) {
	var commands [][]string
	service := New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string(nil), args...))
		switch strings.Join(args, " ") {
		case "snapshot-list guest-1 --name":
			return []byte("before-update\n"), nil
		case "snapshot-info guest-1 --snapshotname before-update", "snapshot-info guest-1 --snapshotname after-update":
			return []byte("Name: before-update\nState: shutoff\nCreation Time: 2026-09-28 10:00:00 +0200\n"), nil
		case "snapshot-create-as guest-1 --name after-update --description Created by LumoNAS --atomic",
			"snapshot-revert guest-1 --snapshotname before-update",
			"snapshot-delete guest-1 --snapshotname before-update":
			return nil, nil
		default:
			t.Fatalf("unexpected snapshot command: %v", args)
			return nil, nil
		}
	})
	snapshots, err := service.Snapshots(context.Background(), "guest-1")
	if err != nil || len(snapshots) != 1 || snapshots[0].Name != "before-update" || snapshots[0].State != "shutoff" {
		t.Fatalf("snapshot list = %#v, %v", snapshots, err)
	}
	created, err := service.CreateSnapshot(context.Background(), "guest-1", "after-update")
	if err != nil || created.Name != "after-update" {
		t.Fatalf("snapshot creation = %#v, %v", created, err)
	}
	if err := service.RevertSnapshot(context.Background(), "guest-1", "before-update"); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteSnapshot(context.Background(), "guest-1", "before-update"); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 6 {
		t.Fatalf("unexpected snapshot command count: %v", commands)
	}
	if _, err := service.CreateSnapshot(context.Background(), "guest-1;rm", "pwn"); err == nil {
		t.Fatal("unsafe VM name was accepted")
	}
}

func testISOImage() []byte {
	image := make([]byte, 32_775)
	image[32_768] = 1
	copy(image[32_769:], "CD001")
	image[32_774] = 1
	return image
}
