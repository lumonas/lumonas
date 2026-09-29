package main

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/virtualization"
)

type testVirtualMachineConsole struct {
	starts int
	closes int
	input  string
	state  virtualization.ConsoleState
}

func (console *testVirtualMachineConsole) Start(_ context.Context, _, _ string) error {
	console.starts++
	return nil
}
func (console *testVirtualMachineConsole) Read(string, int64) (virtualization.ConsoleState, error) {
	return console.state, nil
}
func (console *testVirtualMachineConsole) Write(_ string, data []byte) error {
	console.input += string(data)
	return nil
}
func (console *testVirtualMachineConsole) Close(string) error { console.closes++; return nil }

func TestVirtualMachineInventoryAndFixedActionAPI(t *testing.T) {
	server := testServer(t)
	var commands []string
	server.virtualizationService = virtualization.New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		commands = append(commands, binary+" "+strings.Join(args, " "))
		switch strings.Join(args, " ") {
		case "uri":
			return []byte("qemu:///system\n"), nil
		case "list --all --name":
			return []byte("media-vm\n"), nil
		case "dominfo media-vm":
			return []byte("Name: media-vm\nUUID: test-uuid\nState: running\nCPU(s): 2\nMax memory: 1048576 KiB\nUsed memory: 512000 KiB\n"), nil
		case "shutdown media-vm":
			return nil, nil
		default:
			t.Fatalf("unexpected command %q", commands[len(commands)-1])
			return nil, nil
		}
	})
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/virtualization/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"libvirtAvailable":true`) {
		t.Fatalf("unexpected host status: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/virtualization/vms", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"media-vm"`) {
		t.Fatalf("unexpected VM inventory: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/virtualization/vms/media-vm/action", strings.NewReader(`{"action":"shutdown"}`)))
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"state":"running"`) {
		t.Fatalf("unexpected VM action: %d %s", response.Code, response.Body.String())
	}
	for _, command := range commands {
		if strings.Contains(command, ";") || strings.Contains(command, "destroy") {
			t.Fatalf("unbounded or destructive command used: %q", command)
		}
	}
}

func TestVirtualMachineMediaUploadAndCreateAPI(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	isoDir, vmDir := filepath.Join(root, "isos"), filepath.Join(root, "vms")
	if err := os.MkdirAll(isoDir, 0750); err != nil {
		t.Fatal(err)
	}
	memInfo := filepath.Join(root, "meminfo")
	if err := os.WriteFile(memInfo, []byte("MemTotal:       8388608 kB\n"), 0600); err != nil {
		t.Fatal(err)
	}
	server.virtualizationService = virtualization.New(func(_ context.Context, binary string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case joined == "uri":
			return []byte("qemu:///system"), nil
		case joined == "list --all --name":
			return nil, nil
		case binary == "qemu-img" && len(args) == 5 && args[0] == "create":
			return nil, os.WriteFile(args[3], []byte("qcow2"), 0600)
		case strings.HasPrefix(joined, "define --validate "), joined == "start e2e-vm":
			return nil, nil
		case joined == "dominfo e2e-vm":
			return []byte("Name: e2e-vm\nState: running\nCPU(s): 1\nMax memory: 524288 KiB\n"), nil
		default:
			t.Fatalf("unexpected virtualization command: %s %v", binary, args)
			return nil, nil
		}
	})
	server.virtualizationService.ISODir = isoDir
	server.virtualizationService.VMDir = vmDir
	server.virtualizationService.MemInfo = memInfo

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "test-install.iso")
	if err != nil {
		t.Fatal(err)
	}
	isoImage := make([]byte, 32_775)
	isoImage[32_768] = 1
	copy(isoImage[32_769:], "CD001")
	isoImage[32_774] = 1
	if _, err := part.Write(isoImage); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/virtualization/media", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"name":"test-install.iso"`) {
		t.Fatalf("unexpected ISO upload: %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/virtualization/media", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"test-install.iso"`) {
		t.Fatalf("unexpected media inventory: %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/virtualization/vms", strings.NewReader(`{"name":"e2e-vm","iso":"test-install.iso","vcpus":1,"memoryMiB":512,"diskGiB":8}`)))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"state":"running"`) {
		t.Fatalf("unexpected VM create response: %d %s", response.Code, response.Body.String())
	}
}

func TestVirtualMachineDeleteRequiresExactNameAndRemovesManagedDisk(t *testing.T) {
	server := testServer(t)
	vmDir := t.TempDir()
	diskPath := filepath.Join(vmDir, "guest-1.qcow2")
	definition := `<domain><name>guest-1</name><devices><disk type="file" device="disk"><source file="` + diskPath + `"/><target dev="vda"/></disk></devices></domain>`
	definitionPath := filepath.Join(vmDir, "guest-1.xml")
	if err := os.WriteFile(definitionPath, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskPath, []byte("qcow2"), 0600); err != nil {
		t.Fatal(err)
	}
	server.virtualizationService = virtualization.New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "domstate guest-1":
			return []byte("shut off\n"), nil
		case "undefine guest-1 --managed-save --snapshots-metadata":
			return nil, nil
		default:
			t.Fatalf("unexpected virtualization command: %v", args)
			return nil, nil
		}
	})
	server.virtualizationService.VMDir = vmDir
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/virtualization/vms/guest-1", strings.NewReader(`{"confirmName":"wrong-name","deleteDisk":true}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "type the VM name exactly") {
		t.Fatalf("wrong confirmation was accepted: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/virtualization/vms/guest-1", strings.NewReader(`{"confirmName":"guest-1","deleteDisk":true}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"diskRemoved":true`) {
		t.Fatalf("managed VM delete failed: %d %s", response.Code, response.Body.String())
	}
	for _, target := range []string{definitionPath, diskPath} {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("managed VM file %q remains: %v", target, err)
		}
	}
}

func TestVirtualMachineSnapshotAPICoversListCreateRestoreAndDelete(t *testing.T) {
	server := testServer(t)
	server.virtualizationService = virtualization.New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "snapshot-list guest-1 --name":
			return []byte("before-upgrade\n"), nil
		case "snapshot-info guest-1 --snapshotname before-upgrade":
			return []byte("State: shutoff\nCreation Time: 2026-09-28 10:00:00 +0200\n"), nil
		case "snapshot-create-as guest-1 --name before-upgrade --description Created by LumoNAS --atomic",
			"snapshot-revert guest-1 --snapshotname before-upgrade",
			"snapshot-delete guest-1 --snapshotname before-upgrade":
			return nil, nil
		default:
			t.Fatalf("unexpected snapshot command: %v", args)
			return nil, nil
		}
	})

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/virtualization/vms/guest-1/snapshots", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"before-upgrade"`) {
		t.Fatalf("unexpected snapshot list: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/virtualization/vms/guest-1/snapshots", strings.NewReader(`{"name":"before-upgrade"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("unexpected snapshot create: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/virtualization/vms/guest-1/snapshots/before-upgrade/revert", strings.NewReader(`{}`)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("unexpected snapshot revert: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/virtualization/vms/guest-1/snapshots/before-upgrade", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected snapshot delete: %d %s", response.Code, response.Body.String())
	}
}

func TestVirtualMachineSerialConsoleAPIRequiresRunningVMAndSupportsInputClose(t *testing.T) {
	server := testServer(t)
	console := &testVirtualMachineConsole{state: virtualization.ConsoleState{Output: "guest booted", Cursor: 11, Connected: true}}
	server.virtualizationService = virtualization.New(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") != "domstate guest-1" {
			return nil, errors.New("unexpected libvirt command")
		}
		return []byte("running\n"), nil
	})
	server.virtualizationService.ConsoleRuntime = console

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/virtualization/vms/guest-1/console?cursor=0", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"output":"guest booted"`) || console.starts != 1 {
		t.Fatalf("unexpected console open/read: %d %s starts=%d", response.Code, response.Body.String(), console.starts)
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/virtualization/vms/guest-1/console", strings.NewReader(`{"data":"\u001b[A"}`)))
	if response.Code != http.StatusAccepted || console.input != "\x1b[A" {
		t.Fatalf("unexpected console input: %d %s input=%q", response.Code, response.Body.String(), console.input)
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/virtualization/vms/guest-1/console", nil))
	if response.Code != http.StatusNoContent || console.closes != 1 {
		t.Fatalf("unexpected console close: %d closes=%d", response.Code, console.closes)
	}
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/virtualization/vms/guest-1/console?cursor=-1", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("negative console cursor was accepted: %d", response.Code)
	}
	console.starts = 0
	server.virtualizationService.Run = func(context.Context, string, ...string) ([]byte, error) { return []byte("shut off\n"), nil }
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/virtualization/vms/guest-1/console", nil))
	if response.Code != http.StatusConflict || console.starts != 0 {
		t.Fatalf("console opened on stopped VM: %d %s", response.Code, response.Body.String())
	}
}
