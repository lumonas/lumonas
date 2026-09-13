package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFirewallActivationFailsClosedWithoutBroker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nftables.conf")
	previous := "table inet lumonas {\n\tchain input {}\n}\n"
	if err := os.WriteFile(path, []byte(previous), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_FIREWALL_CONFIG", path)
	t.Setenv("LUMONAS_PRIVD_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))

	server := &apiServer{}
	if _, err := server.activateFirewallRules(context.Background(), "table inet lumonas {\n\tchain input { policy drop; }\n}\n"); err == nil {
		t.Fatal("firewall activation without the privileged broker unexpectedly succeeded")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != previous {
		t.Fatalf("firewall configuration changed after broker failure: %q", data)
	}
}
