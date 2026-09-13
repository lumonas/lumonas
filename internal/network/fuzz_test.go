package network

import "testing"

func FuzzWiFiPSKValidation(f *testing.F) {
	f.Add("correct horse battery staple")
	f.Add("\x00\n")
	f.Fuzz(func(t *testing.T, psk string) {
		_ = ValidateWiFiPSK(psk)
	})
}

func FuzzNetworkConnectionValidation(f *testing.F) {
	f.Add("lan", "LAN", "eth0", "ethernet", 1500, false)
	f.Add("bad id", "", "../../etc", "unknown", 1, true)
	f.Fuzz(func(t *testing.T, id, name, iface, kind string, mtu int, wifi bool) {
		connection := Connection{
			ID: id, Name: name, Interface: iface, Type: kind, MTU: mtu,
			SSID: "fuzz-ssid", WiFiOpen: wifi,
			IPv4: IPConfig{Method: "auto"}, IPv6: IPConfig{Method: "disabled"},
		}
		_ = connection.Validate()
		_, _ = connection.NetworkManagerChanges()
	})
}
