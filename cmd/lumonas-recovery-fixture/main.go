package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/store"
)

func main() {
	bundlePath := flag.String("bundle", "", "output recovery bundle path")
	keyPath := flag.String("key", "", "output recovery key path")
	flag.Parse()
	if *bundlePath == "" || *keyPath == "" {
		fatal("--bundle and --key are required")
	}
	key := []byte("fixture-independent-recovery-key")
	database, err := buildFixtureDatabase()
	if err != nil {
		fatal("create fixture database: %v", err)
	}
	bundle, err := recovery.Create(recovery.Input{
		Manifest: recovery.Manifest{
			ConfigSchema:   1,
			LumoNASVersion: "fixture",
			NASUUID:        "fixture-nas",
			Generation:     2,
			DiskIDs:        []string{"serial:DATA1", "serial:DATA2", "serial:PARITY"},
		},
		DesiredState: []byte(`{"nasUuid":"fixture-nas","hostname":"recovered-nas","generation":2,"shares":["share-media"],"users":["operator","media"],"dockerStacks":["media"],"protection":{"parityDiskId":"serial:PARITY","dataDiskIds":["serial:DATA1","serial:DATA2"]}}`),
		Database:     database,
		Compose: map[string][]byte{
			"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n"),
		},
		Files: map[string][]byte{
			"config/shares.json":            []byte("[{\"id\":\"share-media\",\"name\":\"Media\",\"path\":\"/srv/media\",\"enabled\":true,\"protocols\":[\"smb\",\"nfs\"]}]\n"),
			"config/users.json":             []byte("[{\"name\":\"operator\",\"kind\":\"user\",\"managementRole\":\"admin\"},{\"name\":\"media\",\"kind\":\"user\",\"managementRole\":\"none\"}]\n"),
			"config/config-generation.json": []byte("{\"generation\":2,\"state\":\"committed\",\"planHash\":\"fixture-plan-2\"}\n"),
			"storage/snapraid.conf":         []byte("parity /srv/disks/serial_PARITY/snapraid.parity\ncontent /var/lib/lumonas/snapraid.content\ndata d1 /srv/disks/serial_DATA1\ndata d2 /srv/disks/serial_DATA2\n"),
			"storage/mergerfs.conf":         []byte("/srv/disks/serial_DATA1:/srv/disks/serial_DATA2 /srv/pools/media fuse.mergerfs defaults,allow_other,use_ino 0 0\n"),
			"storage/disk-identities.json":  []byte("[{\"id\":\"serial:DATA1\",\"role\":\"data\"},{\"id\":\"serial:DATA2\",\"role\":\"data\"},{\"id\":\"serial:PARITY\",\"role\":\"parity\"}]\n"),
			"acl/share-media.json":          []byte("{\"shareId\":\"share-media\",\"principal\":\"media\",\"level\":\"write\"}\n"),
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

func buildFixtureDatabase() ([]byte, error) {
	directory, err := os.MkdirTemp("", "lumonas-recovery-fixture-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	database, err := store.Open(filepath.Join(directory, "lumonas.db"))
	if err != nil {
		return nil, err
	}
	defer database.Close()
	operator, err := database.CreatePrincipal(identity.CreateInput{
		Kind: identity.KindUser, Name: "operator", Password: "fixture-long-admin-password", ManagementRole: identity.RoleAdmin,
	})
	if err != nil {
		return nil, err
	}
	media, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "media"})
	if err != nil {
		return nil, err
	}
	group, err := database.CreatePrincipal(identity.CreateInput{Kind: identity.KindGroup, Name: "household"})
	if err != nil {
		return nil, err
	}
	if err := database.SetGroupMembers(group.ID, []string{media.ID}); err != nil {
		return nil, err
	}
	if _, err := database.CreateManagedShare(shares.ManagedShare{
		ID: "share-media", Name: "Media", Path: "/srv/media", Enabled: true,
		Protocols: []shares.Protocol{{Name: "smb"}, {Name: "nfs", Settings: map[string]any{"rootSquash": true}}},
		Access:    []shares.AccessRule{{PrincipalID: media.ID, Level: "write"}},
	}); err != nil {
		return nil, err
	}
	generation, err := database.BeginGeneration("fixture-plan-2")
	if err != nil {
		return nil, err
	}
	if generation != 2 {
		return nil, fmt.Errorf("unexpected fixture generation %d", generation)
	}
	if err := database.CommitGeneration(generation); err != nil {
		return nil, err
	}
	if operator.ID == "" {
		return nil, fmt.Errorf("fixture operator was not assigned an id")
	}
	return database.BackupBytes()
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "lumonas-recovery-fixture: "+format+"\n", args...)
	os.Exit(1)
}
