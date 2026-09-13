package network

import (
	"context"
	"strings"
	"testing"
)

func TestTailscaleMutationsUseInjectedBoundedRunner(t *testing.T) {
	var commands []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	if err := TailscaleUpWithRunner(context.Background(), "lumonas", "auth-secret", run); err != nil {
		t.Fatal(err)
	}
	if err := TailscaleDownWithRunner(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := TailscaleSetExitNodeWithRunner(context.Background(), "100.64.0.2", run); err != nil {
		t.Fatal(err)
	}
	if err := TailscaleClearExitNodeWithRunner(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 || !strings.HasPrefix(commands[0], "tailscale up") || commands[1] != "tailscale down" || commands[2] != "tailscale set --exit-node=100.64.0.2" || commands[3] != "tailscale set --exit-node=none" {
		t.Fatalf("unexpected tailscale command sequence: %#v", commands)
	}
}

func TestValidateTailscaleConfigRejectsEmptyHostname(t *testing.T) {
	if err := ValidateTailscaleConfig(""); err == nil {
		t.Fatal("empty hostname should fail")
	}
}

func TestValidateTailscaleConfigRejectsTooLong(t *testing.T) {
	long := ""
	for i := 0; i < 64; i++ {
		long += "a"
	}
	if err := ValidateTailscaleConfig(long); err == nil {
		t.Fatal("64 character hostname should fail")
	}
}

func TestValidateTailscaleConfigRejectsUppercase(t *testing.T) {
	if err := ValidateTailscaleConfig("MyHost"); err == nil {
		t.Fatal("uppercase hostname should fail")
	}
}

func TestValidateTailscaleConfigRejectsSpecialChars(t *testing.T) {
	if err := ValidateTailscaleConfig("my_host"); err == nil {
		t.Fatal("underscore hostname should fail")
	}
	if err := ValidateTailscaleConfig("my.host"); err == nil {
		t.Fatal("dot hostname should fail")
	}
}

func TestValidateTailscaleConfigAcceptsValid(t *testing.T) {
	valid := []string{"lumonas", "nas-01", "server123", "a", "my-nas-server"}
	for _, h := range valid {
		if err := ValidateTailscaleConfig(h); err != nil {
			t.Fatalf("hostname %q rejected: %v", h, err)
		}
	}
}

func TestValidateTailscaleConfigAcceptsMaxLength(t *testing.T) {
	maxLen := ""
	for i := 0; i < 63; i++ {
		maxLen += "a"
	}
	if err := ValidateTailscaleConfig(maxLen); err != nil {
		t.Fatalf("63 character hostname should be accepted: %v", err)
	}
}

func TestTailscaleIsInstalled(t *testing.T) {
	_ = TailscaleIsInstalled()
	// No assertion — just ensure it doesn't panic
}

func TestTailscalePeersRequiresTailscale(t *testing.T) {
	if !TailscaleIsInstalled() {
		t.Skip("tailscale not installed")
	}
	ctx := context.Background()
	_, err := TailscalePeers(ctx)
	// May fail if not authenticated, but should not panic
	if err != nil {
		t.Logf("TailscalePeers returned error (expected if not authed): %v", err)
	}
}
