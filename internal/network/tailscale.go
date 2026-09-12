package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type TailscaleStatus struct {
	Running        bool     `json:"running"`
	Connected      bool     `json:"connected"`
	Health         string   `json:"health,omitempty"`
	BackendState   string   `json:"backendState"`
	Version        string   `json:"version,omitempty"`
	TailscaleIP4   string   `json:"tailscaleIp4,omitempty"`
	TailscaleIP6   string   `json:"tailscaleIp6,omitempty"`
	HostName       string   `json:"hostName"`
	MagicDNSSuffix string   `json:"magicDnsSuffix,omitempty"`
	ExitNode       string   `json:"exitNode,omitempty"`
	ExitNodeAllow  bool     `json:"exitNodeAllow"`
	SubnetRoutes   []string `json:"subnetRoutes"`
}

type TailscalePeer struct {
	HostName    string `json:"hostName"`
	TailscaleIP string `json:"tailscaleIp"`
	PublicKey   string `json:"publicKey"`
	OS          string `json:"os,omitempty"`
	Online      bool   `json:"online"`
	ExitNode    bool   `json:"exitNode"`
}

type TailscaleDNSName struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

func TailscaleGetStatus(ctx context.Context) (*TailscaleStatus, error) {
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}
	status := &TailscaleStatus{
		BackendState: "Running",
		HostName:     "lumonas",
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "\"BackendState\":") {
			status.BackendState = strings.Trim(strings.TrimPrefix(line, "\"BackendState\":"), " \"")
		}
		if strings.HasPrefix(line, "\"Version\":") {
			status.Version = strings.Trim(strings.TrimPrefix(line, "\"Version\":"), " \"")
		}
	}
	if status.BackendState == "Running" {
		status.Running = true
	}
	return status, nil
}

func TailscaleIsInstalled() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

func TailscaleUp(ctx context.Context, hostname string, authKey string) error {
	args := []string{"up", "--hostname=" + hostname}
	if authKey != "" {
		args = append(args, "--authkey="+authKey)
	}
	if out, err := exec.CommandContext(ctx, "tailscale", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tailscale up: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func TailscaleDown(ctx context.Context) error {
	if out, err := exec.CommandContext(ctx, "tailscale", "down").CombinedOutput(); err != nil {
		return fmt.Errorf("tailscale down: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func TailscaleSetExitNode(ctx context.Context, peerIP string) error {
	args := []string{"set", "--exit-node=" + peerIP}
	if out, err := exec.CommandContext(ctx, "tailscale", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tailscale set exit-node: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func TailscaleClearExitNode(ctx context.Context) error {
	if out, err := exec.CommandContext(ctx, "tailscale", "set", "--exit-node=none").CombinedOutput(); err != nil {
		return fmt.Errorf("tailscale clear exit-node: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func TailscaleAdvertiseSubnet(ctx context.Context, cidr string) error {
	if out, err := exec.CommandContext(ctx, "tailscale", "set", "--advertise-routes="+cidr).CombinedOutput(); err != nil {
		return fmt.Errorf("tailscale advertise subnet: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func TailscaleDisableSubnet(ctx context.Context) error {
	if out, err := exec.CommandContext(ctx, "tailscale", "set", "--advertise-routes=").CombinedOutput(); err != nil {
		return fmt.Errorf("tailscale disable subnet: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

type tailscaleJSON struct {
	BackendState string
	Self         struct {
		HostName    string `json:"HostName"`
		TailscaleIP string `json:"TailscaleIPs"`
	}
	Peer map[string]struct {
		HostName    string `json:"HostName"`
		TailscaleIP string `json:"TailscaleIPs"`
		OS          string `json:"OS"`
		Online      *bool  `json:"Online"`
		ExitNode    bool   `json:"ExitNode"`
		PublicKey   string `json:"PublicKey"`
	} `json:"Peer"`
}

func TailscalePeers(ctx context.Context) ([]TailscalePeer, error) {
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}
	var status tailscaleJSON
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("tailscale status json: %w", err)
	}
	peers := make([]TailscalePeer, 0, len(status.Peer))
	for _, p := range status.Peer {
		online := false
		if p.Online != nil {
			online = *p.Online
		}
		ip := p.TailscaleIP
		if idx := strings.Index(ip, "/"); idx != -1 {
			ip = ip[:idx]
		}
		peers = append(peers, TailscalePeer{
			HostName:    p.HostName,
			TailscaleIP: ip,
			PublicKey:   p.PublicKey,
			OS:          p.OS,
			Online:      online,
			ExitNode:    p.ExitNode,
		})
	}
	return peers, nil
}

func ValidateTailscaleConfig(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if len(hostname) > 63 {
		return fmt.Errorf("hostname must be 63 characters or fewer")
	}
	for _, c := range hostname {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return fmt.Errorf("hostname must contain only lowercase letters, numbers, and hyphens")
		}
	}
	return nil
}
