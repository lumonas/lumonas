package main

import (
	"strings"
	"testing"
	"time"
)

func TestNetworkWakeOperationSendsPacket(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	request := request{
		Operation:      "network.wol.wake",
		OperationID:    "op-wake",
		PlanHash:       "hash",
		RequestedState: map[string]any{"interface": "eth0", "mac": "AA:BB:CC:DD:EE:FF"},
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
		Confirmed:      true,
	}
	result := execute(request, nil, run)
	if !result.OK {
		t.Fatalf("unexpected failure: %s", result.Error)
	}
	if len(commands) != 1 || commands[0] != "etherwake -i eth0 aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected wake command: %v", commands)
	}
}

func TestNetworkWakeOperationRejectsBadTargets(t *testing.T) {
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	request := request{
		Operation:      "network.wol.wake",
		OperationID:    "op-wake",
		PlanHash:       "hash",
		RequestedState: map[string]any{"interface": "../escape", "mac": "aa:bb:cc:dd:ee:ff"},
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
		Confirmed:      true,
	}
	if result := execute(request, nil, run); result.OK {
		t.Fatal("invalid interface must be rejected")
	}

	request.RequestedState = map[string]any{"interface": "eth0", "mac": "not-a-mac"}
	if result := execute(request, nil, run); result.OK {
		t.Fatal("invalid MAC must be rejected")
	}

	request.Confirmed = false
	request.RequestedState = map[string]any{"interface": "eth0", "mac": "aa:bb:cc:dd:ee:ff"}
	if result := execute(request, nil, run); result.OK {
		t.Fatal("unconfirmed wake must be rejected")
	}
}

func TestNetworkWakeOperationRequiresOperationID(t *testing.T) {
	if !requiresOperationID("network.wol.wake") {
		t.Fatal("wake is a mutation and must require an operation ID")
	}
	if operationWorker("network.wol.wake") != "network" {
		t.Fatal("wake must route to the network worker")
	}
}
