package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/privileged"
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

func TestReconcileUpdateBootArmsPreviousSlotBeforeAutomaticReboot(t *testing.T) {
	server := testServer(t)
	server.log = slog.Default()
	server.version = "2.0.0"
	root := t.TempDir()
	t.Setenv("LUMONAS_UPDATE_ROOT", root)
	t.Setenv("LUMONAS_SLOT_DEVICES", "a=/dev/disk/by-partlabel/lumonas-a:0001,b=/dev/disk/by-partlabel/lumonas-b:0002")
	state := updates.SlotState{ActiveSlot: "a", PendingSlot: "b", PendingVersion: "2.0.0", BootAttempts: updates.DefaultMaxBootAttempts - 1}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	var requests []privileged.Request
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		requests = append(requests, request)
		return privileged.Response{OK: true}, nil
	}

	server.reconcileUpdateBoot()
	if len(requests) != 2 {
		t.Fatalf("automatic rollback did not issue BootNext and reboot: %#v", requests)
	}
	if requests[0].Operation != "system.slot.bootnext" || requests[0].RequestedState["entry"] != "0001" {
		t.Fatalf("unexpected BootNext rollback request: %#v", requests[0])
	}
	if requests[1].Operation != "power.shutdown" || requests[1].RequestedState["action"] != "reboot" {
		t.Fatalf("unexpected reboot rollback request: %#v", requests[1])
	}
	if requests[0].OperationID == "" || requests[0].OperationID != requests[1].OperationID || requests[0].PlanHash != requests[1].PlanHash {
		t.Fatalf("rollback requests lost shared correlation: %#v", requests)
	}
}
