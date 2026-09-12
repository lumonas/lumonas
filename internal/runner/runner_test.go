package runner

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestOutputContextHonorsParentDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := OutputContext(ctx, "sh", "-c", "sleep 10 & wait")
	if err == nil || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("command was not bounded by parent context: err=%v elapsed=%s", err, time.Since(started))
	}
}

func TestOutputContextPreservesStdoutAndStderrOnFailure(t *testing.T) {
	out, err := OutputContext(context.Background(), "sh", "-c", "printf 'stdout'; printf 'stderr' >&2; exit 7")
	if err == nil || string(out) != "stdout" {
		t.Fatalf("expected stdout and failure, output=%q err=%v", out, err)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || !strings.Contains(string(exitErr.Stderr), "stderr") {
		t.Fatalf("expected captured stderr, err=%#v", err)
	}
}

func TestCombinedOutputPreservesCommandDiagnostics(t *testing.T) {
	out, err := CombinedOutput("sh", "-c", "printf 'diagnostic' >&2; exit 7")
	if err == nil || !strings.Contains(string(out), "diagnostic") {
		t.Fatalf("expected bounded command diagnostics, output=%q err=%v", out, err)
	}
}

func TestCombinedOutputContextKillsDescendantsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := CombinedOutputContext(ctx, "sh", "-c", "sleep 10 & wait")
	if err == nil {
		t.Fatal("expected command cancellation")
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("command group outlived cancellation: elapsed=%s err=%v", elapsed, err)
	}
}

func TestOutputContextRejectsExcessiveOutput(t *testing.T) {
	_, err := OutputContext(context.Background(), "sh", "-c", "head -c 1048577 /dev/zero")
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("expected output limit error, got %v", err)
	}
}

func TestCombinedOutputRejectsExcessiveOutput(t *testing.T) {
	_, err := CombinedOutput("sh", "-c", "head -c 1048577 /dev/zero")
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("expected output limit error, got %v", err)
	}
}
