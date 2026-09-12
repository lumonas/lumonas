package main

import (
	"context"
	"os/exec"
	"testing"
	"time"

	commandrunner "github.com/lumonas/lumonas/internal/runner"
)

func TestWaitProcessGroupKillsNetworkCheckpointDescendants(t *testing.T) {
	command := exec.Command("sh", "-c", "sleep 10 & wait")
	commandrunner.ConfigureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := waitProcessGroup(ctx, command); err == nil {
		t.Fatal("expected the checkpoint process to be terminated")
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("checkpoint descendants outlived timeout: %s", elapsed)
	}
}
