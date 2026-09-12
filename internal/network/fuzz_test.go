package network

import "testing"

func FuzzWiFiPSKValidation(f *testing.F) {
	f.Add("correct horse battery staple")
	f.Add("\x00\n")
	f.Fuzz(func(t *testing.T, psk string) {
		_ = ValidateWiFiPSK(psk)
	})
}
