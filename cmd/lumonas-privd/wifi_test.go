package main

import (
	"strings"
	"testing"
)

func TestConnectWiFiValidatesPlanAndInputs(t *testing.T) {
	base := request{Operation: "network.wifi.connect", OperationID: "wifi-1", PlanHash: "hash", RequestedState: map[string]any{"ssid": "HomeNet", "psk": "correct horse battery staple"}, Confirmed: true}
	run := func(name string, args []string, stdin string) ([]byte, error) { return nil, nil }

	unconfirmed := base
	unconfirmed.Confirmed = false
	if result := connectWiFi(unconfirmed, run); result.OK {
		t.Fatal("unconfirmed plan should fail")
	}

	badSSID := base
	badSSID.RequestedState = map[string]any{"ssid": strings.Repeat("x", 33), "psk": "correct horse battery staple"}
	if result := connectWiFi(badSSID, run); result.OK {
		t.Fatal("over-long ssid should fail")
	}

	badDevice := base
	badDevice.RequestedState = map[string]any{"ssid": "HomeNet", "ifname": "bad;device", "psk": "correct horse battery staple"}
	if result := connectWiFi(badDevice, run); result.OK {
		t.Fatal("invalid device name should fail")
	}

	badPSK := base
	badPSK.RequestedState = map[string]any{"ssid": "HomeNet", "psk": "short"}
	if result := connectWiFi(badPSK, run); result.OK {
		t.Fatal("short psk should fail")
	}

	badTimeout := base
	badTimeout.RequestedState = map[string]any{"ssid": "HomeNet", "psk": "correct horse battery staple", "timeoutSeconds": 10}
	if result := connectWiFi(badTimeout, run); result.OK {
		t.Fatal("timeout below 30 should fail")
	}
}

func TestConnectWiFiPassesPasswordThroughStdinNotArgv(t *testing.T) {
	var seenArgs []string
	var seenStdin string
	run := func(name string, args []string, stdin string) ([]byte, error) {
		seenArgs, seenStdin = args, stdin
		return nil, nil
	}
	request := request{
		Operation: "network.wifi.connect", OperationID: "wifi-2", PlanHash: "hash",
		RequestedState: map[string]any{"ssid": "HomeNet", "ifname": "wlan0", "psk": "correct horse battery staple", "timeoutSeconds": 45},
		Confirmed:      true,
	}
	result := connectWiFi(request, run)
	if !result.OK {
		t.Fatalf("expected success, got %#v", result)
	}
	joined := strings.Join(seenArgs, " ")
	if strings.Contains(joined, "correct horse battery staple") {
		t.Fatalf("password leaked into argv: %q", joined)
	}
	if seenStdin != "correct horse battery staple\n" {
		t.Fatalf("expected password on stdin, got %q", seenStdin)
	}
	for _, want := range []string{"--wait", "45", "--ask", "device", "wifi", "connect", "HomeNet", "ifname", "wlan0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in argv %q", want, joined)
		}
	}
}

func TestConnectWiFiOpenNetworkOmitsSecrets(t *testing.T) {
	var seenArgs []string
	var seenStdin string
	run := func(name string, args []string, stdin string) ([]byte, error) {
		seenArgs, seenStdin = args, stdin
		return nil, nil
	}
	request := request{
		Operation: "network.wifi.connect", OperationID: "wifi-3", PlanHash: "hash",
		RequestedState: map[string]any{"ssid": "FreeGuest"}, Confirmed: true,
	}
	if result := connectWiFi(request, run); !result.OK {
		t.Fatalf("expected success, got %#v", result)
	}
	for _, arg := range seenArgs {
		if arg == "--" {
			t.Fatalf("open network should not pass a secret terminator: %#v", seenArgs)
		}
	}
	if seenStdin != "" {
		t.Fatalf("open network should not feed stdin, got %q", seenStdin)
	}
}
