package collector

import (
	"errors"
	"testing"
)

func TestDisksPromoteMountedPartitionMetadata(t *testing.T) {
	output := []byte(`{"blockdevices":[{"name":"sda","path":"/dev/sda","type":"disk","size":100,"ptuuid":"disk-guid","children":[{"name":"sda1","path":"/dev/sda1","type":"part","fstype":"ext4","uuid":"fs-uuid","partuuid":"part-uuid","mountpoint":"/srv/data"}]}]}`)
	disks, err := Disks(func(name string, args ...string) ([]byte, error) {
		if name != "lsblk" {
			return nil, errors.New("optional collector unavailable")
		}
		return output, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 {
		t.Fatalf("expected one physical disk, got %#v", disks)
	}
	disk := disks[0]
	if disk.ID != "gpt:disk-guid" || disk.FilesystemUUID != "fs-uuid" || disk.PartitionUUID != "part-uuid" || disk.Filesystem != "ext4" || !disk.Mounted {
		t.Fatalf("partition metadata was not promoted safely: %#v", disk)
	}
}

func TestMountedPartitionWinsOverUnmountedFilesystemChild(t *testing.T) {
	disk := inheritFilesystemMetadata(lsblkDevice{Children: []lsblkDevice{
		{FSType: "ext4", UUID: "first", PartUUID: "part-first"},
		{FSType: "xfs", UUID: "mounted", PartUUID: "part-mounted", Mountpoint: "/srv/pool"},
	}})
	if disk.UUID != "mounted" || disk.FSType != "xfs" || disk.PartUUID != "part-mounted" || disk.Mountpoint != "/srv/pool" {
		t.Fatalf("mounted child did not win filesystem selection: %#v", disk)
	}
}
