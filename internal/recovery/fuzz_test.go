package recovery

import "testing"

func FuzzVerifyBundle(f *testing.F) {
	f.Add([]byte("not a zip bundle"), []byte("recovery-key"))
	f.Add([]byte{}, []byte{})
	f.Fuzz(func(t *testing.T, bundle, key []byte) {
		_, _ = Verify(bundle, key)
	})
}

func FuzzPlanBundle(f *testing.F) {
	f.Add([]byte("not a zip bundle"), []byte("recovery-key"))
	f.Fuzz(func(t *testing.T, bundle, key []byte) {
		_, _ = Plan(bundle, key)
	})
}
