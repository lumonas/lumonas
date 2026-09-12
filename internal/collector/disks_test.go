package collector

import (
	"testing"
)

func TestDisksUseStableIdentityAcrossDevicePathChanges(t *testing.T) {
	first := []byte(`{"blockdevices":[{"name":"sdb","path":"/dev/sdb","type":"disk","size":100,"model":"Test","serial":"SERIAL-1","wwn":"wwn-1","rota":true,"tran":"sata"}]}`)
	second := []byte(`{"blockdevices":[{"name":"sdc","path":"/dev/sdc","type":"disk","size":100,"model":"Test","serial":"SERIAL-1","wwn":"wwn-1","rota":true,"tran":"sata"}]}`)
	left, err := Disks(func(string, ...string) ([]byte, error) { return first, nil })
	if err != nil {
		t.Fatal(err)
	}
	right, err := Disks(func(string, ...string) ([]byte, error) { return second, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || len(right) != 1 || left[0].ID != right[0].ID {
		t.Fatalf("expected stable identity, got %#v and %#v", left, right)
	}
	if left[0].ID != "wwn:wwn-1" {
		t.Fatalf("unexpected identity %q", left[0].ID)
	}
}

func TestStableIDFallsBackInSafeOrder(t *testing.T) {
	if got := StableID(lsblkDevice{Path: "/dev/sda", Serial: "S"}); got != "serial:S" {
		t.Fatal(got)
	}
	if got := StableID(lsblkDevice{Path: "/dev/sda", UUID: "U"}); got != "uuid:U" {
		t.Fatal(got)
	}
	if got := StableID(lsblkDevice{Path: "/dev/sda"}); got != "path:/dev/sda" {
		t.Fatal(got)
	}
}
