//go:build linux

package virtualization

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxSerialConsoleUsesPTYAndCarriesOutputAndInput(t *testing.T) {
	program := filepath.Join(t.TempDir(), "fake-virsh")
	script := "#!/bin/sh\nprintf 'console ready\\r\\n'\nIFS= read -r line\nprintf 'received:%s\\r\\n' \"$line\"\n"
	if err := os.WriteFile(program, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := newConsoleRuntime()
	if err := runtime.Start(context.Background(), program, "guest-1"); err != nil {
		t.Fatal(err)
	}
	defer runtime.Close("guest-1")

	state := waitForConsoleText(t, runtime, "guest-1", "console ready")
	if err := runtime.Write("guest-1", []byte("help\r")); err != nil {
		t.Fatal(err)
	}
	state = waitForConsoleText(t, runtime, "guest-1", "received:help")
	if !strings.Contains(state.Output, "console ready") || !strings.Contains(state.Output, "received:help") {
		t.Fatalf("serial console output did not include expected text: %q", state.Output)
	}
}

func waitForConsoleText(t *testing.T, runtime ConsoleRuntime, name, want string) ConsoleState {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, err := runtime.Read(name, 0)
		if err == nil && strings.Contains(state.Output, want) {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, err := runtime.Read(name, 0)
	t.Fatalf("timed out waiting for %q in console output %q (err=%v)", want, state.Output, err)
	return ConsoleState{}
}
