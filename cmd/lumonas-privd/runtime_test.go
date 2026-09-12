package main

import (
	"strings"
	"testing"
)

// fakeCommand serves canned outputs keyed by a substring of the command.
type fakeCommand map[string]string

func (f fakeCommand) run(name string, args ...string) ([]byte, error) {
	joined := strings.Join(append([]string{name}, args...), " ")
	for needle, output := range f {
		if strings.Contains(joined, needle) {
			return []byte(output), nil
		}
	}
	return nil, nil
}

func TestRuntimeStatusReadsKernelInterfaces(t *testing.T) {
	run := fakeCommand{
		"/proc/swaps":   "Filename\t\t\t\tType\t\tSize\tUsed\tPriority\n/dev/zram0 partition\t1048576\t262144\t100\n",
		"/proc/meminfo": "MemTotal:        8000000 kB\nMemAvailable:    3000000 kB\n",
		"findmnt":       "tmpfs 536870912\n",
	}
	response := runtimeStatus(run.run)
	if !response.OK {
		t.Fatalf("runtime.status failed: %s", response.Error)
	}
	data := response.Data.(map[string]any)
	zram := data["zram"].(map[string]any)
	if zram["enabled"] != true || zram["sizeBytes"].(int64) != 1073741824 {
		t.Fatalf("zram state wrong: %#v", zram)
	}
	if zram["compressedBytes"].(int64) != 268435456 {
		t.Fatalf("compressed bytes wrong: %#v", zram)
	}
	if zram["ratio"].(float64) != 4.0 {
		t.Fatalf("ratio wrong: %#v", zram)
	}
	tmpfs := data["tmpfs"].(map[string]any)
	if tmpfs["enabled"] != true || tmpfs["sizeBytes"].(int64) != 536870912 {
		t.Fatalf("tmpfs state wrong: %#v", tmpfs)
	}
}

func TestRuntimeStatusWithoutZramReportsDisabled(t *testing.T) {
	run := fakeCommand{
		"/proc/swaps": "Filename\t\t\t\tType\t\tSize\tUsed\tPriority\n",
	}
	response := runtimeStatus(run.run)
	if !response.OK {
		t.Fatalf("runtime.status failed: %s", response.Error)
	}
	data := response.Data.(map[string]any)
	zram := data["zram"].(map[string]any)
	if zram["enabled"] != false {
		t.Fatalf("zram should be disabled: %#v", zram)
	}
}

func TestApplyZramRejectsSizesOutsideBounds(t *testing.T) {
	req := request{Confirmed: true, RequestedState: map[string]any{"sizeBytes": float64(1024)}}
	if response := applyZram(req, fakeCommand{}.run); response.OK {
		t.Fatal("tiny zram was accepted")
	}
	if response := applyZram(request{RequestedState: map[string]any{"sizeBytes": float64(1024)}}, fakeCommand{}.run); response.OK || response.Error != "operation plan is not confirmed" {
		t.Fatalf("unconfirmed apply must be rejected, got %#v", response)
	}
}

func TestApplyZramIdempotentWhenActive(t *testing.T) {
	run := fakeCommand{
		"/proc/swaps": "Filename\t\t\t\tType\t\tSize\tUsed\tPriority\n/dev/zram0 partition\t1048576\t0\t100\n",
	}
	req := request{Confirmed: true, RequestedState: map[string]any{"sizeBytes": float64(2147483648)}}
	response := applyZram(req, run.run)
	if !response.OK {
		t.Fatalf("idempotent apply failed: %s", response.Error)
	}
	data := response.Data.(map[string]any)
	if data["state"] != "unchanged" {
		t.Fatalf("expected unchanged state, got %#v", response.Data)
	}
}

func TestApplyTmpfsRejectsUnconfirmedAndBadSizes(t *testing.T) {
	if response := applyTmpfs(request{}, fakeCommand{}.run); response.OK {
		t.Fatal("unconfirmed tmpfs apply was accepted")
	}
	req := request{Confirmed: true, RequestedState: map[string]any{"sizeBytes": float64(128 * 1024 * 1024 * 1024)}}
	if response := applyTmpfs(req, fakeCommand{}.run); response.OK {
		t.Fatal("oversized tmpfs was accepted")
	}
}
