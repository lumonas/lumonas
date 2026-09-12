package store

import (
	"testing"

	"github.com/lumonas/lumonas/internal/network"
)

func TestNetworkConfigurationPersistsAndDefaultsSafely(t *testing.T) {
	database, err := Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	connections, err := database.ListNetworkConnections()
	if err != nil || len(connections) != 0 {
		t.Fatalf("unexpected initial connections: %#v err=%v", connections, err)
	}
	value, err := database.UpsertNetworkConnection(network.Connection{ID: "lan", UUID: "12345678-1234", Name: "LAN", Interface: "en0", Enabled: true, IPv4: network.IPConfig{Method: "auto"}, IPv6: network.IPConfig{Method: "disabled"}})
	if err != nil {
		t.Fatal(err)
	}
	if value.ID != "lan" {
		t.Fatalf("unexpected connection: %#v", value)
	}
	bindings, err := database.ListNetworkBindings()
	if err != nil || len(bindings) == 0 {
		t.Fatalf("expected safe default bindings: %#v err=%v", bindings, err)
	}
	policy, err := database.NetworkFirewallPolicy()
	if err != nil || policy.Default != "deny" {
		t.Fatalf("unexpected firewall default: %#v err=%v", policy, err)
	}
	policy.Services["ui"] = network.FirewallService{LAN: true, Tailscale: false}
	if _, err := database.SaveNetworkFirewallPolicy(policy); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordNetworkCheckpoint("net-1", "lan", "pending"); err != nil {
		t.Fatal(err)
	}
	connectionID, err := database.CompleteNetworkCheckpoint("net-1", "commit")
	if err != nil || connectionID != "lan" {
		t.Fatalf("checkpoint mapping failed: %q err=%v", connectionID, err)
	}
}
