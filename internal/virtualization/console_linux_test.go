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

// consoleWaitBudget bounds the wait for a round trip that spawns a process and
// exchanges data through a pseudo-terminal. That is real IPC with real
// scheduling, so its latency is not bounded by anything the test controls: the
// usual round trip is well under a second, but a loaded CI runner has been
// observed to take over three, which is what a 3s deadline turned into an
// intermittent failure of a passing test. The assertions are unchanged; only
// how long a slow machine is given to satisfy them is.
const consoleWaitBudget = 15 * time.Second

func waitForConsoleText(t *testing.T, runtime ConsoleRuntime, name, want string) ConsoleState {
	t.Helper()
	started := time.Now()
	deadline := started.Add(consoleWaitBudget)
	for time.Now().Before(deadline) {
		state, err := runtime.Read(name, 0)
		if err == nil && strings.Contains(state.Output, want) {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, err := runtime.Read(name, 0)
	// Report what was actually seen, and for how long, so a genuine hang is
	// distinguishable from a slow machine in the log.
	t.Fatalf("timed out after %s waiting for %q in console output %q (err=%v)",
		time.Since(started).Round(time.Millisecond), want, state.Output, err)
	return ConsoleState{}
}
