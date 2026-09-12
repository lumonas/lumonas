package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lumonas/lumonas/internal/recovery"
)

func main() {
	bundlePath := flag.String("bundle", "", "output recovery bundle path")
	keyPath := flag.String("key", "", "output recovery key path")
	flag.Parse()
	if *bundlePath == "" || *keyPath == "" {
		fatal("--bundle and --key are required")
	}
	key := []byte("fixture-independent-recovery-key")
	bundle, err := recovery.Create(recovery.Input{
		Manifest: recovery.Manifest{
			ConfigSchema:   1,
			LumoNASVersion: "fixture",
			NASUUID:        "fixture-nas",
			Generation:     42,
			DiskIDs:        []string{"serial:DATA1", "serial:DATA2"},
		},
		DesiredState: []byte(`{"nasUuid":"fixture-nas","hostname":"recovered-nas"}`),
		Database:     []byte("SQLite format 3\x00fixture database"),
		Compose: map[string][]byte{
			"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n"),
		},
		Files: map[string][]byte{
			"config/shares.json":    []byte("[{\"name\":\"Media\"}]\n"),
			"storage/snapraid.conf": []byte("parity /srv/pools/parity\n"),
		},
		EncryptedData: []byte("fixture-encrypted-secret"),
	}, key)
	if err != nil {
		fatal("create fixture bundle: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*bundlePath), 0o750); err != nil {
		fatal("create bundle directory: %v", err)
	}
	if err := os.WriteFile(*bundlePath, bundle, 0o600); err != nil {
		fatal("write bundle: %v", err)
	}
	if err := os.WriteFile(*keyPath, key, 0o600); err != nil {
		fatal("write key: %v", err)
	}
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "lumonas-recovery-fixture: "+format+"\n", args...)
	os.Exit(1)
}
