package collector

import (
	"strings"
	"testing"
)

func TestDisksPopulateGPTIdentityAndUseItBeforeFilesystemUUID(t *testing.T) {
	var commandArgs []string
	output := []byte(`{"blockdevices":[{"name":"sda","path":"/dev/sda","type":"disk","size":100,"serial":"","wwn":"","ptuuid":"disk-guid-1","uuid":"filesystem-1","rota":true,"tran":"sata"}]}`)
	disks, err := Disks(func(name string, args ...string) ([]byte, error) {
		if name == "lsblk" {
			commandArgs = append([]string{name}, args...)
		}
		return output, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 || disks[0].GPTDiskGUID != "disk-guid-1" || disks[0].ID != "gpt:disk-guid-1" {
		t.Fatalf("unexpected GPT identity: %#v", disks)
	}
	if !strings.Contains(strings.Join(commandArgs, " "), "PTUUID") {
		t.Fatalf("lsblk command did not request PTUUID: %v", commandArgs)
	}
}
