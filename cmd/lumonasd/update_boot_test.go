package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/updates"
)

func TestReconcileUpdateBootClearsStaleCandidate(t *testing.T) {
	server := testServer(t)
	server.log = slog.Default()
	server.version = "2.0.0"
	root := t.TempDir()
	t.Setenv("LUMONAS_UPDATE_ROOT", root)
	state := updates.SlotState{
		ActiveSlot:     "a",
		PendingSlot:    "b",
		PendingVersion: "2.0.0",
		BootAttempts:   updates.DefaultMaxBootAttempts - 1,
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	server.reconcileUpdateBoot()
	updated, err := (&updates.Manager{Root: root}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if updated.PendingSlot != "" || updated.BootAttempts != 0 || updated.ActiveSlot != "a" || updated.LastError == "" {
		t.Fatalf("candidate was not failed closed: %#v", updated)
	}
}
