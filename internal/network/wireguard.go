package network

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type WireGuardPeer struct {
	PublicKey    string   `json:"publicKey"`
	PresharedKey string   `json:"presharedKey,omitempty"`
	Endpoint     string   `json:"endpoint,omitempty"`
	AllowedIPs   []string `json:"allowedIps"`
	PersistentKeepalive int `json:"persistentKeepalive"`
}

type WireGuardConfig struct {
	Interface   string            `json:"interface"`
	PrivateKey  string            `json:"privateKey,omitempty"`
	Address     []string          `json:"address"`
	ListenPort  int               `json:"listenPort"`
	DNS         []string          `json:"dns,omitempty"`
	Peers       []WireGuardPeer   `json:"peers"`
	PostUp      string            `json:"postUp,omitempty"`
	PostDown    string            `json:"postDown,omitempty"`
}

type WireGuardStatus struct {
	Interface string `json:"interface"`
	IP        string `json:"ip"`
	ListenPort int  `json:"listenPort"`
	Peers     int    `json:"peers"`
	Connected bool   `json:"connected"`
}

func GenerateWireGuardKeyPair() (string, string, error) {
	privKey, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", "", fmt.Errorf("wg genkey: %w", err)
	}
	priv := strings.TrimSpace(string(privKey))
	pubKey, err := exec.Command("wg", "pubkey").StdinPipe()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey pipe: %w", err)
	}
	if _, err := pubKey.Write([]byte(priv)); err != nil {
		return "", "", fmt.Errorf("wg pubkey write: %w", err)
	}
	pubKey.Close()
	pub, err := exec.Command("wg", "pubkey").Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey: %w", err)
	}
	return priv, strings.TrimSpace(string(pub)), nil
}

func GeneratePresharedKey() (string, error) {
	out, err := exec.Command("wg", "genpsk").Output()
	if err != nil {
		return "", fmt.Errorf("wg genpsk: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func RenderWireGuardConfig(cfg WireGuardConfig) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[Interface]\n")
	fmt.Fprintf(&sb, "PrivateKey = %s\n", cfg.PrivateKey)
	for _, addr := range cfg.Address {
		fmt.Fprintf(&sb, "Address = %s\n", addr)
	}
	if cfg.ListenPort > 0 {
		fmt.Fprintf(&sb, "ListenPort = %d\n", cfg.ListenPort)
	}
	if len(cfg.DNS) > 0 {
		fmt.Fprintf(&sb, "DNS = %s\n", strings.Join(cfg.DNS, ", "))
	}
	if cfg.PostUp != "" {
		fmt.Fprintf(&sb, "PostUp = %s\n", cfg.PostUp)
	}
	if cfg.PostDown != "" {
		fmt.Fprintf(&sb, "PostDown = %s\n", cfg.PostDown)
	}
	for _, peer := range cfg.Peers {
		fmt.Fprintf(&sb, "\n[Peer]\n")
		fmt.Fprintf(&sb, "PublicKey = %s\n", peer.PublicKey)
		if peer.PresharedKey != "" {
			fmt.Fprintf(&sb, "PresharedKey = %s\n", peer.PresharedKey)
		}
		if peer.Endpoint != "" {
			fmt.Fprintf(&sb, "Endpoint = %s\n", peer.Endpoint)
		}
		if len(peer.AllowedIPs) > 0 {
			fmt.Fprintf(&sb, "AllowedIPs = %s\n", strings.Join(peer.AllowedIPs, ", "))
		}
		if peer.PersistentKeepalive > 0 {
			fmt.Fprintf(&sb, "PersistentKeepalive = %d\n", peer.PersistentKeepalive)
		}
	}
	return sb.String()
}

func ValidateWireGuardConfig(cfg WireGuardConfig) error {
	if cfg.PrivateKey == "" {
		return fmt.Errorf("private key is required")
	}
	if len(cfg.Address) == 0 {
		return fmt.Errorf("at least one address is required")
	}
	for _, addr := range cfg.Address {
		if _, _, err := net.ParseCIDR(addr); err != nil {
			return fmt.Errorf("invalid address %q: %w", addr, err)
		}
	}
	if cfg.ListenPort < 0 || cfg.ListenPort > 65535 {
		return fmt.Errorf("listen port must be between 0 and 65535")
	}
	for i, peer := range cfg.Peers {
		if peer.PublicKey == "" {
			return fmt.Errorf("peer %d: public key is required", i)
		}
		if peer.Endpoint != "" {
			host, port, err := net.SplitHostPort(peer.Endpoint)
			if err != nil {
				return fmt.Errorf("peer %d: invalid endpoint %q: %w", i, peer.Endpoint, err)
			}
			if ip := net.ParseIP(host); ip == nil {
				if _, err := net.LookupHost(host); err != nil {
					return fmt.Errorf("peer %d: cannot resolve endpoint host %q", i, host)
				}
			}
			_ = port
		}
		for _, cidr := range peer.AllowedIPs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("peer %d: invalid allowed IP %q: %w", i, cidr, err)
			}
		}
	}
	return nil
}

func ApplyWireGuardConfig(ctx context.Context, iface string, cfg WireGuardConfig) error {
	if err := ValidateWireGuardConfig(cfg); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "wg", "set", iface, "private-key", "/dev/stdin")
	cmd.Stdin = strings.NewReader(cfg.PrivateKey)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg set: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func WireGuardShow(ctx context.Context, iface string) (*WireGuardStatus, error) {
	out, err := exec.CommandContext(ctx, "wg", "show", iface).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("wg show: %w", err)
	}
	status := &WireGuardStatus{Interface: iface}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "listening port:") {
			fmt.Sscanf(strings.TrimPrefix(line, "listening port:"), "%d", &status.ListenPort)
		}
		if strings.HasPrefix(line, "peer:") {
			status.Peers++
		}
		if strings.HasPrefix(line, "peer:") && strings.Contains(line, "endpoint:") {
			status.Connected = true
		}
	}
	addrs, err := net.InterfaceByName(iface)
	if err == nil {
		addrList, err := addrs.Addrs()
		if err == nil {
			for _, addr := range addrList {
				if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
					status.IP = ipNet.IP.String()
				}
			}
		}
	}
	return status, nil
}

func ValidateWireGuardPeerForQR(peer WireGuardPeer) error {
	if peer.PublicKey == "" {
		return fmt.Errorf("public key is required")
	}
	if len(peer.AllowedIPs) == 0 {
		return fmt.Errorf("at least one allowed IP is required")
	}
	if peer.Endpoint == "" {
		return fmt.Errorf("endpoint is required for QR code")
	}
	return nil
}
