package network

import (
	"strings"
	"testing"
)

func TestConnectionValidationRejectsUnsafeValues(t *testing.T) {
	valid := Connection{ID: "lan", UUID: "12345678-1234", Name: "LAN", Interface: "en0", IPv4: IPConfig{Method: "auto"}, IPv6: IPConfig{Method: "disabled"}, MTU: 1500}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	changes, err := valid.NetworkManagerChanges()
	if err != nil || changes["ipv4.method"] != "auto" || changes["ipv6.method"] != "disabled" {
		t.Fatalf("unexpected NetworkManager changes: %#v err=%v", changes, err)
	}
	invalid := valid
	invalid.Interface = "../../etc"
	if err := invalid.Validate(); err == nil {
		t.Fatal("path traversal interface name should fail")
	}
	invalid = valid
	invalid.IPv4 = IPConfig{Method: "manual", Addresses: []string{"192.168.1.10/24"}, Gateway: "not-an-ip"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid gateway should fail")
	}
	vlan := valid
	vlan.Type, vlan.Parent, vlan.VLANID = "vlan", "en0", 20
	if err := vlan.Validate(); err != nil {
		t.Fatalf("valid VLAN rejected: %v", err)
	}
	invalid = vlan
	invalid.VLANID = 4095
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid VLAN id should fail")
	}
	wifi := Connection{ID: "home-wifi", Name: "Home", Type: "wifi", SSID: "HomeNet", IPv4: IPConfig{Method: "auto"}, IPv6: IPConfig{Method: "disabled"}}
	if err := wifi.Validate(); err != nil {
		t.Fatalf("valid wifi without interface rejected: %v", err)
	}
	wifi.Interface = "wlan0"
	if err := wifi.Validate(); err != nil {
		t.Fatalf("valid wifi rejected: %v", err)
	}
	noSSID := wifi
	noSSID.SSID = ""
	if err := noSSID.Validate(); err == nil {
		t.Fatal("wifi without SSID should fail")
	}
	longSSID := wifi
	longSSID.SSID = strings.Repeat("a", 33)
	if err := longSSID.Validate(); err == nil {
		t.Fatal("33 byte SSID should fail")
	}
	open := wifi
	open.WiFiOpen = true
	if err := open.Validate(); err != nil {
		t.Fatalf("valid open wifi rejected: %v", err)
	}
	openNonWiFi := valid
	openNonWiFi.WiFiOpen = true
	if err := openNonWiFi.Validate(); err == nil {
		t.Fatal("wifiOpen on non-wifi connection should fail")
	}
	ssidOnEthernet := valid
	ssidOnEthernet.SSID = "HomeNet"
	if err := ssidOnEthernet.Validate(); err == nil {
		t.Fatal("SSID on ethernet connection should fail")
	}
	badInterface := wifi
	badInterface.Interface = "../../etc"
	if err := badInterface.Validate(); err == nil {
		t.Fatal("path traversal interface name should fail for wifi too")
	}
	if err := ValidateWiFiPSK("short"); err == nil {
		t.Fatal("short PSK should fail")
	}
	if err := ValidateWiFiPSK(strings.Repeat("x", 64)); err == nil {
		t.Fatal("64 character PSK should fail")
	}
	if err := ValidateWiFiPSK("goodpas sw0rd\n"); err == nil {
		t.Fatal("PSK with control characters should fail")
	}
	if err := ValidateWiFiPSK("goodpassphrase"); err != nil {
		t.Fatalf("valid PSK rejected: %v", err)
	}
	wifiChanges, err := wifi.NetworkManagerChanges()
	if err != nil || wifiChanges["802-11-wireless.ssid"] != "HomeNet" || wifiChanges["802-11-wireless-security.key-mgmt"] != "wpa-psk" {
		t.Fatalf("unexpected wifi NetworkManager changes: %#v err=%v", wifiChanges, err)
	}
	if openChanges, err := open.NetworkManagerChanges(); err != nil || openChanges["802-11-wireless-security.key-mgmt"] != "" {
		t.Fatalf("open wifi should not configure key management: %#v err=%v", openChanges, err)
	}
}

func TestBindingsAndFirewallValidation(t *testing.T) {
	if err := ValidateBindings([]Binding{{Service: "ui", Port: 8080}, {Service: "ui", Port: 8081}}); err == nil {
		t.Fatal("duplicate service binding should fail")
	}
	policy := DefaultFirewallPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	policy.Default = "drop"
	if err := policy.Validate(); err == nil {
		t.Fatal("unsupported firewall default should fail")
	}
	policy = DefaultFirewallPolicy()
	bindings := []Binding{{Service: "ftp", Port: 21, Enabled: true}}
	if err := ValidateExposure(bindings, policy); err == nil {
		t.Fatal("service without an allowed scope should fail")
	}
	policy.Services["ftp"] = FirewallService{LAN: true}
	rules, err := RenderNftables(policy, bindings)
	if err != nil || len(rules) == 0 {
		t.Fatalf("expected generated nftables rules: %q err=%v", rules, err)
	}
	policy.Services["ftp"] = FirewallService{IoT: true}
	if err := ValidateExposure([]Binding{{Service: "ftp", Port: 21, Enabled: true, Scopes: []string{"iot"}}}, policy); err != nil {
		t.Fatalf("valid IoT exposure rejected: %v", err)
	}
}
