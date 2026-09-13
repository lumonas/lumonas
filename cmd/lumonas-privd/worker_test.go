package main

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/privileged"
)

func TestBrokerRequiresConfirmationBeforeForwarding(t *testing.T) {
	result := executeBroker(request{Operation: "filesystem.format", PlanHash: "plan-1"})
	if result.OK || result.Error != "operation plan is not confirmed" {
		t.Fatalf("unexpected broker response: %#v", result)
	}
}

func TestWorkerRejectsOperationsOutsideItsCapabilityDomain(t *testing.T) {
	result := executeWorker(request{Operation: "power.action", OperationID: "power-1", PlanHash: "plan-1", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute)}, "storage")
	if result.OK || result.Error != "operation is not allow-listed for this worker" {
		t.Fatalf("unexpected worker response: %#v", result)
	}
}

func TestWireGuardMutationUsesNetworkWorkerAndTypedConfig(t *testing.T) {
	previous := wireGuardApply
	t.Cleanup(func() { wireGuardApply = previous })
	var gotInterface string
	var gotKey string
	wireGuardApply = func(_ context.Context, iface string, config network.WireGuardConfig, _ network.WireGuardApplyRunner) error {
		gotInterface, gotKey = iface, config.PrivateKey
		return nil
	}
	result := executeWorker(request{
		Operation: "network.wireguard.apply", OperationID: "wireguard-1", PlanHash: "wireguard-plan", Confirmed: true,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
		RequestedState: map[string]any{
			"interface": "wg0",
			"config":    map[string]any{"interface": "wg0", "privateKey": "secret", "address": []any{"10.0.0.1/24"}},
		},
	}, "network")
	if !result.OK || gotInterface != "wg0" || gotKey != "secret" {
		t.Fatalf("unexpected WireGuard worker result: %#v interface=%q key=%q", result, gotInterface, gotKey)
	}
}

func TestTailscaleMutationUsesNetworkWorker(t *testing.T) {
	previous := tailscaleUp
	t.Cleanup(func() { tailscaleUp = previous })
	var gotHostname string
	tailscaleUp = func(_ context.Context, hostname, authKey string, _ network.TailscaleCommandRunner) error {
		gotHostname = hostname
		if authKey != "secret" {
			t.Fatalf("unexpected auth key: %q", authKey)
		}
		return nil
	}
	result := executeWorker(request{
		Operation: "network.tailscale.up", OperationID: "tailscale-1", PlanHash: "tailscale-plan", Confirmed: true,
		ExpiresAt: time.Now().UTC().Add(time.Minute), RequestedState: map[string]any{"hostname": "lumonas", "authKey": "secret"},
	}, "network")
	if !result.OK || gotHostname != "lumonas" {
		t.Fatalf("unexpected Tailscale worker result: %#v hostname=%q", result, gotHostname)
	}
}

func TestBrokerForwardsConfirmedOperationToRootOnlyWorker(t *testing.T) {
	client, server := net.Pipe()
	previousDial := workerDial
	workerDial = func(_ string) (net.Conn, error) { return client, nil }
	t.Cleanup(func() { workerDial = previousDial })
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		var received request
		if err := json.NewDecoder(server).Decode(&received); err != nil {
			done <- err
			return
		}
		if received.Operation != "power.action" {
			done <- &workerTestError{message: "unexpected operation"}
			return
		}
		done <- json.NewEncoder(server).Encode(response{OK: true})
	}()
	result := executeBroker(request{Operation: "power.action", OperationID: "power-1", PlanHash: "plan-1", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute)})
	if !result.OK {
		t.Fatalf("expected worker response, got %#v", result)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not receive forwarded request")
	}
}

func TestBrokerRejectsOversizedWorkerResponse(t *testing.T) {
	client, server := net.Pipe()
	previousDial := workerDial
	workerDial = func(_ string) (net.Conn, error) { return client, nil }
	t.Cleanup(func() { workerDial = previousDial })
	go func() {
		defer server.Close()
		var received request
		if err := json.NewDecoder(server).Decode(&received); err != nil {
			return
		}
		_, _ = server.Write([]byte(`{"ok":true,"data":"` + strings.Repeat("x", privileged.MaxIPCMessageBytes) + `"}` + "\n"))
	}()

	result := executeBroker(request{Operation: "power.action", OperationID: "power-large", PlanHash: "plan-large", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute)})
	if result.OK || result.Error != "privileged worker response failed" {
		t.Fatalf("expected oversized worker response rejection, got %#v", result)
	}
}

type workerTestError struct{ message string }

func (e *workerTestError) Error() string { return e.message }
