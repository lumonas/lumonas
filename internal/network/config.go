package network

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

type IPConfig struct {
	Method    string   `json:"method"`
	Addresses []string `json:"addresses,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
}

type Connection struct {
	ID        string   `json:"id"`
	UUID      string   `json:"uuid"`
	Name      string   `json:"name"`
	Interface string   `json:"interface"`
	Enabled   bool     `json:"enabled"`
	IPv4      IPConfig `json:"ipv4"`
	IPv6      IPConfig `json:"ipv6"`
	MTU       int      `json:"mtu,omitempty"`
}

type Binding struct {
	Service string `json:"service"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

type FirewallService struct {
	LAN       bool `json:"lan"`
	Tailscale bool `json:"tailscale"`
}

type FirewallPolicy struct {
	Enabled  bool                       `json:"enabled"`
	Default  string                     `json:"default"`
	Services map[string]FirewallService `json:"services"`
}

var connectionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var interfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

func (c Connection) Validate() error {
	if !connectionIDPattern.MatchString(c.ID) {
		return errors.New("network connection id is invalid")
	}
	if c.UUID != "" && !regexp.MustCompile(`^[a-fA-F0-9-]{8,64}$`).MatchString(c.UUID) {
		return errors.New("network connection uuid is invalid")
	}
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 128 {
		return errors.New("network connection name is required")
	}
	if !interfacePattern.MatchString(c.Interface) {
		return errors.New("network interface name is invalid")
	}
	if c.MTU != 0 && (c.MTU < 576 || c.MTU > 9000) {
		return errors.New("network MTU must be between 576 and 9000")
	}
	if err := c.IPv4.validate(4); err != nil {
		return fmt.Errorf("ipv4: %w", err)
	}
	if err := c.IPv6.validate(6); err != nil {
		return fmt.Errorf("ipv6: %w", err)
	}
	return nil
}

func (c IPConfig) validate(version int) error {
	switch c.Method {
	case "auto", "manual", "disabled":
	default:
		return fmt.Errorf("method %q is unsupported", c.Method)
	}
	if c.Method == "manual" && len(c.Addresses) == 0 {
		return errors.New("manual configuration requires an address")
	}
	if c.Method != "manual" && (len(c.Addresses) > 0 || c.Gateway != "") {
		return errors.New("addresses and gateway require manual configuration")
	}
	for _, value := range c.Addresses {
		ip, _, err := net.ParseCIDR(strings.TrimSpace(value))
		if err != nil || ip.To4() != nil && version != 4 || ip.To4() == nil && version != 6 {
			return fmt.Errorf("address %q is invalid", value)
		}
	}
	if c.Gateway != "" {
		ip := net.ParseIP(strings.TrimSpace(c.Gateway))
		if ip == nil || ip.To4() != nil && version != 4 || ip.To4() == nil && version != 6 {
			return fmt.Errorf("gateway %q is invalid", c.Gateway)
		}
	}
	for _, value := range c.DNS {
		if net.ParseIP(strings.TrimSpace(value)) == nil {
			return fmt.Errorf("DNS server %q is invalid", value)
		}
	}
	return nil
}

var knownServices = map[string]bool{
	"ui": true, "ssh": true, "smb": true, "nfs": true, "sftp": true,
	"ftp": true, "rsync": true, "reverse-proxy": true,
}

func ValidateBindings(values []Binding) error {
	seen := map[string]bool{}
	for _, value := range values {
		if !knownServices[value.Service] {
			return fmt.Errorf("unsupported service binding %q", value.Service)
		}
		if seen[value.Service] {
			return fmt.Errorf("service %q is bound more than once", value.Service)
		}
		seen[value.Service] = true
		if value.Port < 1 || value.Port > 65535 {
			return fmt.Errorf("service %q port must be between 1 and 65535", value.Service)
		}
		if value.Address != "" && net.ParseIP(value.Address) == nil && value.Address != "0.0.0.0" && value.Address != "::" {
			return fmt.Errorf("service %q bind address is invalid", value.Service)
		}
	}
	return nil
}

func DefaultBindings() []Binding {
	return []Binding{{Service: "ui", Address: "0.0.0.0", Port: 8080, Enabled: true}, {Service: "ssh", Port: 22, Enabled: true}, {Service: "smb", Port: 445, Enabled: true}}
}

func (p FirewallPolicy) Validate() error {
	if p.Default != "allow" && p.Default != "deny" {
		return errors.New("firewall default must be allow or deny")
	}
	for service := range p.Services {
		if !knownServices[service] {
			return fmt.Errorf("unsupported firewall service %q", service)
		}
	}
	return nil
}

func DefaultFirewallPolicy() FirewallPolicy {
	return FirewallPolicy{Enabled: true, Default: "deny", Services: map[string]FirewallService{"ui": {LAN: true, Tailscale: true}, "ssh": {LAN: true}, "smb": {LAN: true}, "nfs": {LAN: true}}}
}
