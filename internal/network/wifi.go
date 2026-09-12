package network

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CommandRunner executes an external command so tests can substitute canned
// output.
type CommandRunner func(name string, args ...string) ([]byte, error)

// SystemRunner runs commands through the OS.
func SystemRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// WiFiNetwork is a single entry from a NetworkManager scan.
type WiFiNetwork struct {
	SSID     string `json:"ssid"`
	Signal   int    `json:"signal"`
	Channel  int    `json:"channel"`
	Band     string `json:"band,omitempty"`
	Security string `json:"security"`
	Secure   bool   `json:"secure"`
}

// ScanWiFi lists visible Wi-Fi networks using nmcli. The second return value
// reports whether Wi-Fi management is available at all: false means no nmcli
// binary or no Wi-Fi device (for example on macOS development hosts), with a
// nil error so callers can degrade to a manual SSID entry instead of failing.
func ScanWiFi(run CommandRunner) ([]WiFiNetwork, bool, error) {
	if run == nil {
		if _, err := exec.LookPath("nmcli"); err != nil {
			return []WiFiNetwork{}, false, nil
		}
		run = SystemRunner
	}
	out, err := run("nmcli", "-t", "-f", "SSID,SIGNAL,FREQ,CHAN,SECURITY", "device", "wifi", "list")
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "no wi-fi device found") || strings.Contains(message, "no wifi device found") {
			return []WiFiNetwork{}, false, nil
		}
		return nil, true, fmt.Errorf("nmcli wifi scan: %w", err)
	}
	seen := make(map[string]bool)
	networks := make([]WiFiNetwork, 0, 8)
	for _, line := range strings.Split(string(out), "\n") {
		fields := splitTerse(line)
		if len(fields) < 5 {
			continue
		}
		ssid := fields[0]
		if ssid == "" || ssid == "--" {
			continue
		}
		signal, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		band, _ := frequencyBand(strings.TrimSpace(fields[2]))
		channel, _ := strconv.Atoi(strings.TrimSpace(fields[3]))
		security := strings.TrimSpace(fields[4])
		if seen[ssid] {
			continue
		}
		seen[ssid] = true
		networks = append(networks, WiFiNetwork{SSID: ssid, Signal: signal, Channel: channel, Band: band, Security: security, Secure: security != ""})
	}
	return networks, true, nil
}

func frequencyBand(value string) (string, bool) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", false
	}
	mhz, err := strconv.Atoi(fields[0])
	if err != nil {
		return "", false
	}
	if mhz >= 5935 {
		return "6 GHz", true
	}
	if mhz >= 4900 {
		return "5 GHz", true
	}
	return "2.4 GHz", true
}

// splitTerse splits a nmcli terse-mode line on colons, honouring the backslash
// escaping nmcli applies to literal colons inside values.
func splitTerse(line string) []string {
	fields := make([]string, 0, 5)
	current := strings.Builder{}
	escaped := false
	for _, char := range line {
		switch {
		case escaped:
			if char == ':' {
				current.WriteRune(':')
			} else {
				current.WriteRune('\\')
				current.WriteRune(char)
			}
			escaped = false
		case char == '\\':
			escaped = true
		case char == ':':
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	return append(fields, current.String())
}
