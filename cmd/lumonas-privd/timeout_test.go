package main

import (
	"testing"
	"time"
)

func TestCommandRunnerHasAnExecutionDeadline(t *testing.T) {
	previous := privilegedCommandTimeout
	privilegedCommandTimeout = 20 * time.Millisecond
	t.Cleanup(func() { privilegedCommandTimeout = previous })
	started := time.Now()
	_, err := commandRunner("sh", "-c", "sleep 1")
	if err == nil || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("privileged command was not bounded: err=%v elapsed=%s", err, time.Since(started))
	}
}

func TestStdinCommandRunnerHasAnExecutionDeadline(t *testing.T) {
	previous := privilegedCommandTimeout
	privilegedCommandTimeout = 20 * time.Millisecond
	t.Cleanup(func() { privilegedCommandTimeout = previous })
	started := time.Now()
	_, err := stdinCommandRunner("sh", []string{"-c", "cat; sleep 1"}, "input")
	if err == nil || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("expected bounded stdin command failure, got %v", err)
	}
}
