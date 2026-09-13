package model

import "testing"

func TestHasStableDiskIdentityRejectsKernelPathFallback(t *testing.T) {
	for _, id := range []string{"", "path:/dev/sda", " path:/dev/sdb "} {
		if HasStableDiskIdentity(id) {
			t.Fatalf("expected unstable disk identity %q to be rejected", id)
		}
	}
	for _, id := range []string{"wwn:123", "serial:abc", "gpt:def", "uuid:ghi"} {
		if !HasStableDiskIdentity(id) {
			t.Fatalf("expected stable disk identity %q to be accepted", id)
		}
	}
}
