package network

import (
	"os"
	"testing"
)

func TestWriteNftablesRejectsNonGeneratedContentAndPreservesPrevious(t *testing.T) {
	path := t.TempDir() + "/nftables.conf"
	if err := os.WriteFile(path, []byte("working"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteNftables(path, "nft add rule inet filter input accept"); err == nil {
		t.Fatal("raw nftables content should be rejected")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "working" {
		t.Fatalf("previous ruleset was not preserved: %q err=%v", data, err)
	}
}
