package network

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestApplyWireGuardConfigUsesBoundedCommandRunner(t *testing.T) {
	original := runWireGuardCommand
	t.Cleanup(func() { runWireGuardCommand = original })
	var gotCommand string
	var gotArgs []string
	var gotStdin string
	runWireGuardCommand = func(_ context.Context, stdin io.Reader, command string, args ...string) ([]byte, error) {
		data, _ := io.ReadAll(stdin)
		gotCommand, gotArgs, gotStdin = command, args, string(data)
		return nil, nil
	}
	cfg := WireGuardConfig{PrivateKey: "private", Address: []string{"10.0.0.1/24"}}
	if err := ApplyWireGuardConfig(context.Background(), "wg0", cfg); err != nil {
		t.Fatal(err)
	}
	if gotCommand != "wg" || strings.Join(gotArgs, " ") != "set wg0 private-key /dev/stdin" || gotStdin != "private" {
		t.Fatalf("unexpected wireguard command: %q %v stdin=%q", gotCommand, gotArgs, gotStdin)
	}
}

func TestApplyWireGuardConfigWithRunnerPreservesPrivateKeyOnStdin(t *testing.T) {
	var gotStdin string
	err := ApplyWireGuardConfigWithRunner(context.Background(), "wg0", WireGuardConfig{PrivateKey: "private", Address: []string{"10.0.0.1/24"}}, func(_ context.Context, stdin io.Reader, _ string, _ ...string) ([]byte, error) {
		data, _ := io.ReadAll(stdin)
		gotStdin = string(data)
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotStdin != "private" {
		t.Fatalf("private key was not sent through stdin: %q", gotStdin)
	}
}

func TestValidateWireGuardConfigRejectsMissingPrivateKey(t *testing.T) {
	cfg := WireGuardConfig{
		Address:    []string{"10.0.0.1/24"},
		ListenPort: 51820,
	}
	if err := ValidateWireGuardConfig(cfg); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("expected private key error, got: %v", err)
	}
}

func TestValidateWireGuardConfigRejectsMissingAddress(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "dGhpcyBpcyBhIHRlc3Qga2V5",
		ListenPort: 51820,
	}
	if err := ValidateWireGuardConfig(cfg); err == nil || !strings.Contains(err.Error(), "address") {
		t.Fatalf("expected address error, got: %v", err)
	}
}

func TestValidateWireGuardConfigRejectsInvalidAddress(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "dGhpcyBpcyBhIHRlc3Qga2V5",
		Address:    []string{"not-a-cidr"},
		ListenPort: 51820,
	}
	if err := ValidateWireGuardConfig(cfg); err == nil || !strings.Contains(err.Error(), "invalid address") {
		t.Fatalf("expected invalid address error, got: %v", err)
	}
}

func TestValidateWireGuardConfigRejectsBadPort(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "dGhpcyBpcyBhIHRlc3Qga2V5",
		Address:    []string{"10.0.0.1/24"},
		ListenPort: 99999,
	}
	if err := ValidateWireGuardConfig(cfg); err == nil || !strings.Contains(err.Error(), "listen port") {
		t.Fatalf("expected listen port error, got: %v", err)
	}
}

func TestValidateWireGuardConfigRejectsMissingPeerPublicKey(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "dGhpcyBpcyBhIHRlc3Qga2V5",
		Address:    []string{"10.0.0.1/24"},
		ListenPort: 51820,
		Peers: []WireGuardPeer{
			{AllowedIPs: []string{"10.0.0.2/32"}},
		},
	}
	if err := ValidateWireGuardConfig(cfg); err == nil || !strings.Contains(err.Error(), "public key") {
		t.Fatalf("expected public key error, got: %v", err)
	}
}

func TestValidateWireGuardConfigRejectsInvalidAllowedIP(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "dGhpcyBpcyBhIHRlc3Qga2V5",
		Address:    []string{"10.0.0.1/24"},
		ListenPort: 51820,
		Peers: []WireGuardPeer{
			{PublicKey: "abc123", AllowedIPs: []string{"bad-cidr"}},
		},
	}
	if err := ValidateWireGuardConfig(cfg); err == nil || !strings.Contains(err.Error(), "invalid allowed IP") {
		t.Fatalf("expected invalid allowed IP error, got: %v", err)
	}
}

func TestValidateWireGuardConfigAcceptsValidConfig(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "dGhpcyBpcyBhIHRlc3Qga2V5",
		Address:    []string{"10.0.0.1/24"},
		ListenPort: 51820,
		Peers: []WireGuardPeer{
			{
				PublicKey:  "abc123",
				AllowedIPs: []string{"10.0.0.2/32"},
			},
		},
	}
	if err := ValidateWireGuardConfig(cfg); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestRenderWireGuardConfig(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "priv123",
		Address:    []string{"10.0.0.1/24"},
		ListenPort: 51820,
		DNS:        []string{"1.1.1.1"},
		Peers: []WireGuardPeer{
			{
				PublicKey:           "pub456",
				Endpoint:            "1.2.3.4:51820",
				AllowedIPs:          []string{"10.0.0.2/32"},
				PersistentKeepalive: 25,
			},
		},
	}
	result := RenderWireGuardConfig(cfg)
	if !strings.Contains(result, "[Interface]") {
		t.Fatal("missing [Interface] section")
	}
	if !strings.Contains(result, "PrivateKey = priv123") {
		t.Fatal("missing PrivateKey")
	}
	if !strings.Contains(result, "Address = 10.0.0.1/24") {
		t.Fatal("missing Address")
	}
	if !strings.Contains(result, "ListenPort = 51820") {
		t.Fatal("missing ListenPort")
	}
	if !strings.Contains(result, "DNS = 1.1.1.1") {
		t.Fatal("missing DNS")
	}
	if !strings.Contains(result, "[Peer]") {
		t.Fatal("missing [Peer] section")
	}
	if !strings.Contains(result, "PublicKey = pub456") {
		t.Fatal("missing peer PublicKey")
	}
	if !strings.Contains(result, "Endpoint = 1.2.3.4:51820") {
		t.Fatal("missing peer Endpoint")
	}
	if !strings.Contains(result, "PersistentKeepalive = 25") {
		t.Fatal("missing PersistentKeepalive")
	}
}

func TestRenderWireGuardConfigOmitsOptionalFields(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "priv123",
		Address:    []string{"10.0.0.1/24"},
		ListenPort: 51820,
	}
	result := RenderWireGuardConfig(cfg)
	if strings.Contains(result, "DNS") {
		t.Fatal("DNS should not be rendered when empty")
	}
	if strings.Contains(result, "PostUp") {
		t.Fatal("PostUp should not be rendered when empty")
	}
	if strings.Contains(result, "[Peer]") {
		t.Fatal("[Peer] should not be rendered when no peers")
	}
}

func TestValidateWireGuardPeerForQR(t *testing.T) {
	valid := WireGuardPeer{
		PublicKey:  "abc123",
		AllowedIPs: []string{"10.0.0.2/32"},
		Endpoint:   "1.2.3.4:51820",
	}
	if err := ValidateWireGuardPeerForQR(valid); err != nil {
		t.Fatalf("valid peer rejected: %v", err)
	}
	if err := ValidateWireGuardPeerForQR(WireGuardPeer{}); err == nil {
		t.Fatal("empty peer should fail")
	}
	noAllowed := WireGuardPeer{PublicKey: "abc123", Endpoint: "1.2.3.4:51820"}
	if err := ValidateWireGuardPeerForQR(noAllowed); err == nil {
		t.Fatal("peer without allowed IPs should fail")
	}
	noEndpoint := WireGuardPeer{PublicKey: "abc123", AllowedIPs: []string{"10.0.0.2/32"}}
	if err := ValidateWireGuardPeerForQR(noEndpoint); err == nil {
		t.Fatal("peer without endpoint should fail")
	}
}
