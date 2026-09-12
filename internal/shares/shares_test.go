package shares

import (
	"os"
	"strings"
	"testing"
)

func TestShareStoreAndSambaRendering(t *testing.T) {
	store := Store{Path: t.TempDir() + "/shares.json"}
	input := []Share{{ID: "share-1", Name: "Documents", Path: "/srv/pools/main/Documents", Enabled: true, Protocols: []string{"smb"}, Access: map[string]string{"family": "write"}}}
	if err := store.Save(input); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Path); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatal("share was not persisted")
	}
	config, err := RenderSamba(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, "[Documents]") || !strings.Contains(config, "path = /srv/pools/main/Documents") {
		t.Fatal(config)
	}
}

func TestShareValidationRejectsRelativePath(t *testing.T) {
	if err := Validate(Share{Name: "Bad", Path: "relative", Protocols: []string{"smb"}}); err == nil {
		t.Fatal("relative path should fail")
	}
}
