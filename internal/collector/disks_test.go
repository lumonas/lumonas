package collector

import (
	"reflect"
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

func TestParseUdevPropertiesIgnoresMalformedLines(t *testing.T) {
	got := parseUdevProperties("ID_WWN=wwn-udev\nmalformed\nID_SERIAL_SHORT=serial-udev\n")
	want := map[string]string{"ID_WWN": "wwn-udev", "ID_SERIAL_SHORT": "serial-udev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected udev properties: %#v", got)
	}
}

func TestEnrichFromUdevFillsMissingStableIdentity(t *testing.T) {
	previous := lookupCommand
	lookupCommand = func(string) (string, error) { return "/usr/bin/udevadm", nil }
	t.Cleanup(func() { lookupCommand = previous })
	device := enrichFromUdev(func(name string, args ...string) ([]byte, error) {
		if name != "udevadm" || !reflect.DeepEqual(args, []string{"info", "--query=property", "--name", "/dev/sdb"}) {
			t.Fatalf("unexpected udev command %q %v", name, args)
		}
		return []byte("ID_WWN=wwn-udev\nID_SERIAL_SHORT=serial-udev\nID_MODEL=Bridge Disk\nID_FS_UUID=fs-udev\nID_PART_TABLE_UUID=gpt-udev\nID_BUS=usb\n"), nil
	}, lsblkDevice{Path: "/dev/sdb"})
	if device.WWN != "wwn-udev" || device.Serial != "serial-udev" || device.Model != "Bridge Disk" || device.UUID != "fs-udev" || device.PTUUID != "gpt-udev" || device.Tran != "usb" {
		t.Fatalf("udev identity enrichment incomplete: %#v", device)
	}
}

func TestEnrichFromUdevPreservesLsblkIdentity(t *testing.T) {
	previous := lookupCommand
	lookupCommand = func(string) (string, error) { return "/usr/bin/udevadm", nil }
	t.Cleanup(func() { lookupCommand = previous })
	device := enrichFromUdev(func(string, ...string) ([]byte, error) {
		return []byte("ID_WWN=udev-wwn\nID_SERIAL_SHORT=udev-serial\n"), nil
	}, lsblkDevice{Path: "/dev/sdb", WWN: "lsblk-wwn", Serial: "lsblk-serial"})
	if device.WWN != "lsblk-wwn" || device.Serial != "lsblk-serial" {
		t.Fatalf("lsblk identity was overwritten: %#v", device)
	}
}
