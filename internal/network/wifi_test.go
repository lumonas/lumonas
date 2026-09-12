package network

import (
	"errors"
	"strings"
	"testing"
)

func TestScanWiFiParsesNmcliOutput(t *testing.T) {
	output := strings.Join([]string{
		`HomeNet:82:5180 MHz:36:WPA2`,
		`Café\:: 90:2412 MHz:1:WPA1 WPA2`,
		`Open_Guest:64:2437 MHz:6:`,
		`HomeNet:40:5180 MHz:36:WPA2`,
		`--:55:2462 MHz:11:WPA2`,
		``,
	}, "\n")
	networks, available, err := ScanWiFi(func(name string, args ...string) ([]byte, error) {
		if name != "nmcli" {
			t.Fatalf("unexpected command %q", name)
		}
		return []byte(output), nil
	})
	if err != nil || !available {
		t.Fatalf("expected available scan, got available=%v err=%v", available, err)
	}
	if len(networks) != 3 {
		t.Fatalf("expected 3 networks, got %#v", networks)
	}
	if networks[0].SSID != "HomeNet" || networks[0].Signal != 82 || networks[0].Channel != 36 || networks[0].Band != "5 GHz" || !networks[0].Secure {
		t.Fatalf("unexpected first network %#v", networks[0])
	}
	if networks[1].SSID != "Café:" || networks[1].Signal != 90 {
		t.Fatalf("escaped colon SSID not parsed: %#v", networks[1])
	}
	if networks[2].Secure || networks[2].Band != "2.4 GHz" {
		t.Fatalf("open network misclassified: %#v", networks[2])
	}
}

func TestScanWiFiIsUnavailableWithoutNmcli(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	networks, available, err := ScanWiFi(nil)
	if err != nil || available || networks == nil || len(networks) != 0 {
		t.Fatalf("expected graceful unavailability, got %#v available=%v err=%v", networks, available, err)
	}
}

func TestScanWiFiReportsMissingWiFiDeviceAsUnavailable(t *testing.T) {
	networks, available, err := ScanWiFi(func(string, ...string) ([]byte, error) {
		return nil, errors.New("Error: no Wi-Fi device found")
	})
	if err != nil || available || networks == nil || len(networks) != 0 {
		t.Fatalf("expected graceful unavailability, got %#v available=%v err=%v", networks, available, err)
	}
}

func TestScanWiFiWrapsOtherFailures(t *testing.T) {
	_, available, err := ScanWiFi(func(string, ...string) ([]byte, error) {
		return nil, errors.New("nmcli exited with status 42")
	})
	if err == nil || !available {
		t.Fatalf("expected hard failure, got available=%v err=%v", available, err)
	}
}
