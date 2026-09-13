package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	commandrunner "github.com/lumonas/lumonas/internal/runner"
)

// LanHost is one neighbor observed on the local network. Discovery is
// observation-only: it never sends traffic to other hosts beyond what the
// kernel's neighbor table already contains.
type LanHost struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	State     string `json:"state"`
	Hostname  string `json:"hostname,omitempty"`
}

type LANRunner func(context.Context, string, ...string) ([]byte, error)

var lanMACPattern = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
var lanIPPattern = regexp.MustCompile(`^[0-9a-fA-F.:]+$`)

// Neighbor states that represent a usable mapping. FAILED and INCOMPLETE are
// intentionally excluded: a failed resolution is not a device.
var reportableNeighborStates = map[string]bool{
	"permanent":  true,
	"noarp":      true,
	"reachable":  true,
	"stale":      true,
	"delay":      true,
	"probe":      true,
	"reachable6": true,
	"stale6":     true,
}

// DiscoverLANHosts reads the kernel neighbor table (`ip neigh show`) and
// returns the hosts with a hardware address attached. Entries without an
// lladdr (incomplete or failed resolutions) are skipped.
func DiscoverLANHosts(ctx context.Context, run LANRunner) ([]LanHost, error) {
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return commandrunner.CombinedOutputContext(ctx, name, args...)
		}
	}
	out, err := run(ctx, "ip", "neigh", "show")
	if err != nil {
		return nil, fmt.Errorf("neighbor table unavailable: %w", err)
	}
	return ParseNeighbors(string(out)), nil
}

// ParseNeighbors parses `ip neigh show` output. Lines look like:
//
//	192.168.1.34 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE
//	fe80::1 dev eth0 lladdr aa:bb:cc:dd:ee:ff router STALE
func ParseNeighbors(output string) []LanHost {
	result := make([]LanHost, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		ip := fields[0]
		if net.ParseIP(ip) == nil || !lanIPPattern.MatchString(ip) {
			continue
		}
		var iface, mac, state string
		for index := 1; index < len(fields); index++ {
			switch fields[index] {
			case "dev":
				if index+1 < len(fields) {
					iface = fields[index+1]
					index++
				}
			case "lladdr":
				if index+1 < len(fields) {
					mac = fields[index+1]
					index++
				}
			default:
				state = fields[index]
			}
		}
		if mac == "" || iface == "" {
			continue
		}
		mac = strings.ToLower(mac)
		if !lanMACPattern.MatchString(mac) || !validInterface.MatchString(iface) {
			continue
		}
		if state != "" && !reportableNeighborStates[strings.ToLower(state)] {
			continue
		}
		key := mac + "/" + iface
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, LanHost{MAC: mac, IP: ip, Interface: iface, State: strings.ToLower(state)})
	}
	return result
}

// ValidateWakeTarget checks a Wake-on-LAN target before the privileged
// worker constructs any command.
func ValidateWakeTarget(iface, mac string) error {
	if !validInterface.MatchString(strings.TrimSpace(iface)) {
		return errors.New("network interface name is invalid")
	}
	if !lanMACPattern.MatchString(strings.ToLower(strings.TrimSpace(mac))) {
		return errors.New("MAC address is invalid")
	}
	return nil
}

// WakeHost sends a Wake-on-LAN magic packet for mac via the etherwake
// utility bound to iface. The caller must be the privileged broker; this
// package never constructs a shell command.
func WakeHost(ctx context.Context, iface, mac string, run WOLRunner) error {
	if err := ValidateWakeTarget(iface, mac); err != nil {
		return err
	}
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return commandrunner.CombinedOutputContext(ctx, name, args...)
		}
	}
	if _, err := run(ctx, "etherwake", "-i", iface, strings.ToLower(mac)); err != nil {
		return fmt.Errorf("wake packet for %s via %s failed: %w", mac, iface, err)
	}
	return nil
}
