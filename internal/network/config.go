package network

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type IPConfig struct {
	Method    string   `json:"method"`
	Addresses []string `json:"addresses,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	Metric    int      `json:"metric,omitempty"`
	Routes    []Route  `json:"routes,omitempty"`
}

type Route struct {
	Destination string `json:"destination"`
	Via         string `json:"via,omitempty"`
	Metric      int    `json:"metric,omitempty"`
}

type Connection struct {
	ID         string   `json:"id"`
	UUID       string   `json:"uuid"`
	Name       string   `json:"name"`
	Interface  string   `json:"interface"`
	Enabled    bool     `json:"enabled"`
	Generation int64    `json:"generation"`
	Status     string   `json:"status"`
	Type       string   `json:"type,omitempty"`
	Parent     string   `json:"parent,omitempty"`
	Members    []string `json:"members,omitempty"`
	VLANID     int      `json:"vlanId,omitempty"`
	SSID       string   `json:"ssid,omitempty"`
	WiFiOpen   bool     `json:"wifiOpen,omitempty"`
	IPv4       IPConfig `json:"ipv4"`
	IPv6       IPConfig `json:"ipv6"`
	MTU        int      `json:"mtu,omitempty"`
}

type Binding struct {
	Service string   `json:"service"`
	Address string   `json:"address"`
	Port    int      `json:"port"`
	Enabled bool     `json:"enabled"`
	Scopes  []string `json:"scopes,omitempty"`
}

type FirewallService struct {
	LAN       bool `json:"lan"`
	Tailscale bool `json:"tailscale"`
	IoT       bool `json:"iot"`
}

type FirewallPolicy struct {
	Enabled        bool                       `json:"enabled"`
	Default        string                     `json:"default"`
	Services       map[string]FirewallService `json:"services"`
	GeneratedRules string                     `json:"generatedRules,omitempty"`
}

var connectionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var interfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

func validConnectionType(value string) bool {
	switch value {
	case "ethernet", "wifi", "vlan", "bond", "bridge":
		return true
	default:
		return false
	}
}

// ValidateWiFiPSK checks a transient pre-shared key at apply time. PSKs are
// never persisted on the Connection record.
func ValidateWiFiPSK(psk string) error {
	if len(psk) < 8 || len(psk) > 63 {
		return errors.New("wifi password must be between 8 and 63 characters")
	}
	if strings.ContainsFunc(psk, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errors.New("wifi password contains control characters")
	}
	return nil
}

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
	if c.Interface == "" && c.Type != "wifi" {
		return errors.New("network interface name is required")
	}
	if c.Interface != "" && !interfacePattern.MatchString(c.Interface) {
		return errors.New("network interface name is invalid")
	}
	if c.Type != "" && !validConnectionType(c.Type) {
		return fmt.Errorf("network connection type %q is unsupported", c.Type)
	}
	if c.Type == "wifi" {
		if len(c.SSID) < 1 || len(c.SSID) > 32 {
			return errors.New("wifi connections require an SSID of 1 to 32 bytes")
		}
		if c.WiFiOpen && strings.ContainsRune(c.SSID, 0) {
			return errors.New("wifi SSID contains invalid characters")
		}
	} else if c.SSID != "" || c.WiFiOpen {
		return errors.New("only wifi connections may carry wifi settings")
	}
	if c.Parent != "" && !interfacePattern.MatchString(c.Parent) {
		return errors.New("network connection parent is invalid")
	}
	if c.Type == "vlan" && (c.Parent == "" || c.VLANID < 1 || c.VLANID > 4094) {
		return errors.New("VLAN connections require a parent and VLAN id between 1 and 4094")
	}
	if (c.Type == "bond" || c.Type == "bridge") && len(c.Members) == 0 {
		return fmt.Errorf("%s connections require at least one member", c.Type)
	}
	seenMembers := map[string]bool{}
	for _, member := range c.Members {
		if !interfacePattern.MatchString(member) || seenMembers[member] {
			return errors.New("network connection member is invalid or duplicated")
		}
		seenMembers[member] = true
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

func (c Connection) NetworkManagerChanges() (map[string]string, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	changes := map[string]string{"connection.autoconnect": strconv.FormatBool(c.Enabled)}
	if c.Type != "" {
		changes["connection.type"] = c.Type
	}
	if c.SSID != "" {
		changes["802-11-wireless.ssid"] = c.SSID
		if !c.WiFiOpen {
			changes["802-11-wireless-security.key-mgmt"] = "wpa-psk"
		}
	}
	if c.Parent != "" {
		if c.Type == "vlan" {
			changes["vlan.parent"], changes["vlan.id"] = c.Parent, strconv.Itoa(c.VLANID)
		} else {
			changes["connection.master"] = c.Parent
		}
	}
	if len(c.Members) > 0 {
		changes["connection.members"] = strings.Join(c.Members, ",")
	}
	if c.MTU != 0 {
		changes["802-3-ethernet.mtu"] = strconv.Itoa(c.MTU)
	}
	addIPChanges(changes, "ipv4", c.IPv4)
	addIPChanges(changes, "ipv6", c.IPv6)
	return changes, nil
}

func addIPChanges(changes map[string]string, prefix string, config IPConfig) {
	changes[prefix+".method"] = config.Method
	if len(config.Addresses) > 0 {
		changes[prefix+".addresses"] = strings.Join(config.Addresses, ",")
	}
	if config.Gateway != "" {
		changes[prefix+".gateway"] = config.Gateway
	}
	if len(config.DNS) > 0 {
		changes[prefix+".dns"] = strings.Join(config.DNS, ",")
	}
	if config.Metric > 0 {
		changes[prefix+".route-metric"] = strconv.Itoa(config.Metric)
	}
	if len(config.Routes) > 0 {
		routes := make([]string, 0, len(config.Routes))
		for _, route := range config.Routes {
			value := route.Destination
			if route.Via != "" {
				value += " " + route.Via
			}
			if route.Metric > 0 {
				value += " " + strconv.Itoa(route.Metric)
			}
			routes = append(routes, value)
		}
		changes[prefix+".routes"] = strings.Join(routes, ",")
	}
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
	if c.Metric < 0 || c.Metric > 4294967295 {
		return errors.New("route metric is invalid")
	}
	for _, route := range c.Routes {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(route.Destination)); err != nil {
			return fmt.Errorf("route destination %q is invalid", route.Destination)
		}
		if route.Via != "" && net.ParseIP(strings.TrimSpace(route.Via)) == nil {
			return fmt.Errorf("route gateway %q is invalid", route.Via)
		}
		if route.Metric < 0 || route.Metric > 4294967295 {
			return fmt.Errorf("route metric for %q is invalid", route.Destination)
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
		for _, scope := range value.Scopes {
			switch scope {
			case "lan", "tailscale", "iot":
			default:
				return fmt.Errorf("service %q network scope %q is unsupported", value.Service, scope)
			}
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

func ValidateExposure(bindings []Binding, policy FirewallPolicy) error {
	if err := ValidateBindings(bindings); err != nil {
		return err
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	for _, binding := range bindings {
		if !binding.Enabled {
			continue
		}
		access := policy.Services[binding.Service]
		if len(binding.Scopes) == 0 && !access.LAN && !access.Tailscale && !access.IoT {
			return fmt.Errorf("enabled service %q has no permitted network scope", binding.Service)
		}
		for _, scope := range binding.Scopes {
			allowed := scope == "lan" && access.LAN || scope == "tailscale" && access.Tailscale || scope == "iot" && access.IoT
			if !allowed {
				return fmt.Errorf("service %q binding and firewall disagree for %s", binding.Service, scope)
			}
		}
	}
	return nil
}

func RenderNftables(policy FirewallPolicy, bindings []Binding) (string, error) {
	if err := ValidateExposure(bindings, policy); err != nil {
		return "", err
	}
	services := make([]Binding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Enabled {
			services = append(services, binding)
		}
	}
	sort.Slice(services, func(left, right int) bool { return services[left].Service < services[right].Service })
	policyName := "drop"
	if policy.Default == "allow" {
		policyName = "accept"
	}
	var builder strings.Builder
	builder.WriteString("table inet lumonas {\n  chain input { type filter hook input priority 0; policy ")
	builder.WriteString(policyName)
	builder.WriteString(";\n    ct state established,related accept\n    iifname \"lo\" accept\n")
	for _, binding := range services {
		access := policy.Services[binding.Service]
		if access.LAN {
			builder.WriteString("    tcp dport ")
			builder.WriteString(strconv.Itoa(binding.Port))
			builder.WriteString(" accept # ")
			builder.WriteString(binding.Service)
			builder.WriteString(" LAN\n")
		}
		if access.Tailscale {
			builder.WriteString("    iifname \"tailscale0\" tcp dport ")
			builder.WriteString(strconv.Itoa(binding.Port))
			builder.WriteString(" accept # ")
			builder.WriteString(binding.Service)
			builder.WriteString(" Tailscale\n")
		}
		if access.IoT {
			builder.WriteString("    iifname \"iot0\" tcp dport ")
			builder.WriteString(strconv.Itoa(binding.Port))
			builder.WriteString(" accept # ")
			builder.WriteString(binding.Service)
			builder.WriteString(" IoT\n")
		}
	}
	builder.WriteString("  }\n}\n")
	return builder.String(), nil
}

func DefaultFirewallPolicy() FirewallPolicy {
	return FirewallPolicy{Enabled: true, Default: "deny", Services: map[string]FirewallService{"ui": {LAN: true, Tailscale: true}, "ssh": {LAN: true}, "smb": {LAN: true}, "nfs": {LAN: true}}}
}
