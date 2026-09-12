package diagnostics

import "testing"

func FuzzRedactText(f *testing.F) {
	f.Add(`password="canary" bearer abc.def.ghi`)
	f.Add("private_key: secret")
	f.Fuzz(func(t *testing.T, value string) {
		_ = RedactText(value)
		_, _ = MarshalJSON(map[string]any{"value": value})
	})
}

func FuzzSupportBundleNames(f *testing.F) {
	f.Add("logs/daemon.txt")
	f.Add("../secret.txt")
	f.Fuzz(func(t *testing.T, name string) {
		_, _ = CreateBundle(map[string][]byte{name: []byte("diagnostic")})
	})
}
