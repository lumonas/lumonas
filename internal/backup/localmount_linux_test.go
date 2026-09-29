//go:build linux

package backup

import "testing"

func TestRemovableBackupTargetClassification(t *testing.T) {
	for _, target := range []string{"/media/usb/backups", "/mnt/backup", "/run/media/alex/USB"} {
		if !IsRemovableBackupTarget(target) {
			t.Errorf("expected removable target %q", target)
		}
	}
	for _, target := range []string{"/srv/backups", "/var/lib/lumonas/recovery", "/media-backups"} {
		if IsRemovableBackupTarget(target) {
			t.Errorf("unexpected removable target %q", target)
		}
	}
}

func TestRemovableBackupTargetMustResolveToAnActualMount(t *testing.T) {
	mountInfo := "36 25 8:1 / /media/usb rw,relatime - ext4 /dev/sdb1 rw\n"
	if err := requireMountedBackupTargetFrom("/media/usb/backups", mountInfo); err != nil {
		t.Fatalf("mounted removable target was rejected: %v", err)
	}
	if err := requireMountedBackupTargetFrom("/media/usb/backups", ""); err == nil {
		t.Fatal("unmounted removable target was accepted")
	}
}

func TestRemovableMountInfoEscapingAndNestedMounts(t *testing.T) {
	mountInfo := "36 25 8:1 / /media/usb rw,relatime - ext4 /dev/sdb1 rw\n" +
		"37 36 8:2 / /media/usb/backups rw,relatime - ext4 /dev/sdb2 rw\n" +
		"38 25 8:3 / /media/disk\\040one rw,relatime - ext4 /dev/sdc1 rw\n"
	if err := requireMountedBackupTargetFrom("/media/usb/backups/daily", mountInfo); err != nil {
		t.Fatalf("nested removable mount was rejected: %v", err)
	}
	if err := requireMountedBackupTargetFrom("/media/usb-other/backups", mountInfo); err == nil {
		t.Fatal("prefix-only unrelated mount was accepted")
	}
	if err := requireMountedBackupTargetFrom("/media/disk one/archive", mountInfo); err != nil {
		t.Fatalf("escaped mount path was rejected: %v", err)
	}
}
