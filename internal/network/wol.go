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

// WOLInterface describes the host Wake-on-LAN capability and current mode.
// It deliberately reports support from ethtool instead of assuming that every
// network interface can wake the machine.
type WOLInterface struct {
	Interface string `json:"interface"`
	MAC       string `json:"mac"`
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
}

type WOLRunner func(context.Context, string, ...string) ([]byte, error)

var validInterface = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)

// DiscoverWOL reads interface inventory from the host and probes each device
// with ethtool. A failed probe is represented as unsupported rather than
// optimistic capability, which keeps the UI from offering a mutation that the
// host cannot verify.
func DiscoverWOL(ctx context.Context, run WOLRunner) []WOLInterface {
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return commandrunner.CombinedOutputContext(ctx, name, args...)
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return []WOLInterface{}
	}
	result := make([]WOLInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.HardwareAddr.String() == "" || !validInterface.MatchString(iface.Name) {
			continue
		}
		status := WOLInterface{Interface: iface.Name, MAC: iface.HardwareAddr.String()}
		output, probeErr := run(ctx, "ethtool", iface.Name)
		if probeErr == nil {
			status.Supported, status.Enabled = parseWOL(string(output))
		}
		result = append(result, status)
	}
	return result
}

// SetWOL changes only the ethtool Wake-on-LAN mode. The caller must be the
// privileged broker; this package never constructs a shell command.
func SetWOL(ctx context.Context, iface string, enabled bool, run WOLRunner) error {
	if !validInterface.MatchString(strings.TrimSpace(iface)) {
		return errors.New("network interface name is invalid")
	}
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return commandrunner.CombinedOutputContext(ctx, name, args...)
		}
	}
	mode := "d"
	if enabled {
		mode = "g"
	}
	if _, err := run(ctx, "ethtool", "-s", iface, "wol", mode); err != nil {
		return fmt.Errorf("set Wake-on-LAN on %s: %w", iface, err)
	}
	return nil
}

func parseWOL(output string) (supported, enabled bool) {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Supports Wake-on":
			supported = strings.Contains(valueString(value), "g")
		case "Wake-on":
			enabled = strings.TrimSpace(valueString(value)) == "g"
		}
	}
	return supported, enabled
}

func valueString(value string) string { return strings.TrimSpace(value) }
