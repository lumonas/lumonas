package main

import (
	"strings"
	"testing"
)

func TestWOLOperationUsesTypedCommandAndRequiresOperationID(t *testing.T) {
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		return nil, nil
	}
	request := request{
		Operation:      "network.wol.set",
		OperationID:    "wol-1",
		PlanHash:       "wol-plan",
		RequestedState: map[string]any{"interface": "enp1s0", "enabled": true},
		Confirmed:      true,
	}
	result := execute(request, nil, run)
	if !result.OK || len(calls) != 1 || calls[0] != "ethtool -s enp1s0 wol g" {
		t.Fatalf("unexpected WOL result: %#v calls=%v", result, calls)
	}

	request.OperationID = ""
	result = execute(request, nil, run)
	if result.OK || result.Error != "operationId is required" {
		t.Fatalf("expected operation ID rejection, got %#v", result)
	}
}

func TestWOLOperationRejectsUnsafeInterfaceBeforeCommand(t *testing.T) {
	called := false
	result := execute(request{
		Operation:      "network.wol.set",
		OperationID:    "wol-1",
		PlanHash:       "wol-plan",
		RequestedState: map[string]any{"interface": "enp1s0;touch /tmp/pwned", "enabled": true},
		Confirmed:      true,
	}, nil, func(string, ...string) ([]byte, error) {
		called = true
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "interface name is invalid") || called {
		t.Fatalf("unsafe WOL interface was accepted: %#v called=%v", result, called)
	}
}
