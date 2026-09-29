package collector

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestDisksReadOnlyIdentityAgainstRealDevice runs the production read-only
// collector against the host's real block devices.
//
// The disposable loopback device that the surrounding smoke creates is NOT
// expected here: lsblk reports loopback attachments with type "loop", and the
// collector intentionally surfaces only type "disk". Offering a loopback file
// as a manageable import target would be a bug, not a feature. What matters
// is that every real disk the collector does report carries a stable identity
// and, when formatted, its filesystem UUID.
func TestDisksReadOnlyIdentityAgainstRealDevice(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("LUMONAS_TEST_DISK_PATH"))
	disks, err := Disks(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, disk := range disks {
		if path != "" && disk.CurrentPath == path {
			t.Fatalf("collector offered loopback device %q as a manageable disk", path)
		}
		if disk.ID == "" || strings.HasPrefix(disk.ID, "path:") {
			t.Fatalf("real disk collector did not retain a stable identity: %#v", disk)
		}
		if disk.Filesystem != "" && disk.FilesystemUUID == "" {
			t.Fatalf("real disk collector lost the filesystem UUID: %#v", disk)
		}
	}
	if path != "" && len(disks) == 0 {
		t.Skip("this host exposes no real block devices to verify against")
	}
}

// A loopback attachment is only manageable once it carries a partition table,
// because that is when lsblk starts reporting a stable identity for it. Before
// that it must stay out of the inventory so it cannot be offered as a
// destructive target.
func TestDisksExcludeUnpartitionedLoopbackButKeepPartitioned(t *testing.T) {
	withoutTable := []byte(`{"blockdevices":[{"name":"loop0","path":"/dev/loop0","type":"loop","size":100,"model":"","serial":"","wwn":"","uuid":""}]}`)
	withTable := []byte(`{"blockdevices":[{"name":"loop1","path":"/dev/loop1","type":"loop","size":100,"model":"","serial":"","wwn":"","uuid":"fs-uuid","ptuuid":"gpt-disk-guid"}]}`)

	unpartitioned, err := Disks(func(string, ...string) ([]byte, error) { return withoutTable, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(unpartitioned) != 0 {
		t.Fatalf("unpartitioned loopback device must not be offered: %#v", unpartitioned)
	}

	partitioned, err := Disks(func(string, ...string) ([]byte, error) { return withTable, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(partitioned) != 1 {
		t.Fatalf("partitioned loopback device was dropped: %#v", partitioned)
	}
	if partitioned[0].ID != "gpt:gpt-disk-guid" {
		t.Fatalf("unexpected identity %q", partitioned[0].ID)
	}
}

// A device with no WWN, serial, partition-table GUID, or filesystem UUID has
// no identity that survives a reboot or a device-letter change, so it cannot
// back a storage plan. Hosts expose such placeholder entries (unused network
// block devices, empty multipath slots), and they must not be offered.
func TestDisksExcludeDevicesWithoutStableIdentity(t *testing.T) {
	payload := []byte(`{"blockdevices":[
		{"name":"nbd0","path":"/dev/nbd0","type":"disk","size":0,"model":"","serial":"","wwn":"","uuid":""},
		{"name":"sda","path":"/dev/sda","type":"disk","size":100,"model":"Test","serial":"SERIAL-1","wwn":"","uuid":""}
	]}`)
	disks, err := Disks(func(string, ...string) ([]byte, error) { return payload, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 {
		t.Fatalf("expected only the device with a stable identity, got %#v", disks)
	}
	if disks[0].CurrentPath != "/dev/sda" {
		t.Fatalf("unexpected disk retained: %#v", disks[0])
	}
}

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
