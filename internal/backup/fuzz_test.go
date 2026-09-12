package backup

import "testing"

func FuzzRemoteObjectValidation(f *testing.F) {
	f.Add("generation-1/latest.mrb")
	f.Add("../escape")
	f.Fuzz(func(t *testing.T, object string) {
		_ = validateRemoteObject(object)
		_, _ = safeJoin("/var/lib/lumonas/recovery", object)
	})
}
