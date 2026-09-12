package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/storage"
	"github.com/lumonas/lumonas/internal/store"
)

func TestFixtureDatabaseContainsRecoverableNASState(t *testing.T) {
	data, err := buildFixtureDatabase()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "lumonas.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	principals, err := database.ListPrincipals("")
	if err != nil || len(principals) != 3 {
		t.Fatalf("expected three restored principals, got %d: %v", len(principals), err)
	}
	shares, err := database.ListManagedShares()
	if err != nil || len(shares) != 1 || shares[0].ID != "share-media" {
		t.Fatalf("expected media share in restored database, got %#v: %v", shares, err)
	}
	if database.CurrentGeneration() != 2 {
		t.Fatalf("expected restored generation 2, got %d", database.CurrentGeneration())
	}
	mounts, err := database.MountEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 3 {
		t.Fatalf("expected three restored mount entries, got %d", len(mounts))
	}
	var pool storage.MountEntry
	for _, mount := range mounts {
		if mount.FSType == "fuse.mergerfs" {
			pool = mount
			break
		}
	}
	if pool.TargetID != "media" || pool.Source == "" {
		t.Fatalf("expected restored mergerfs pool, got %#v", pool)
	}
	connections, err := database.ListNetworkConnections()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].ID != "lan" || connections[0].Interface != "eth0" {
		t.Fatalf("expected restored LAN connection mapping, got %#v", connections)
	}
}

func TestFixtureBundlePassesFullRecoveryPlan(t *testing.T) {
	work := t.TempDir()
	bundlePath := filepath.Join(work, "latest.mrb")
	keyPath := filepath.Join(work, "recovery.key")
	key := []byte("fixture-independent-recovery-key")
	database, err := buildFixtureDatabase()
	if err != nil {
		t.Fatal(err)
	}
	appdata, err := buildFixtureAppdata()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := recovery.Create(recovery.Input{
		Manifest:      recovery.Manifest{ConfigSchema: 1, LumoNASVersion: "fixture", NASUUID: "fixture-nas", Generation: 2, DiskIDs: []string{"serial:DATA1", "serial:DATA2", "serial:PARITY"}},
		DesiredState:  []byte(`{"nasUuid":"fixture-nas","generation":2}`),
		Database:      database,
		Compose:       map[string][]byte{"media/compose.yaml": []byte("services:\n  media:\n    image: example/media:latest\n    volumes:\n      - /srv/lumonas/docker/appdata/media:/config\n")},
		Appdata:       []recovery.AppdataPayload{{Stack: "media", ContainerPath: "/config", HostPath: "/srv/lumonas/docker/appdata/media", Archive: appdata}},
		Files:         map[string][]byte{"storage/snapraid.conf": []byte("parity /srv/disks/serial_PARITY/snapraid.parity\n")},
		EncryptedData: []byte("fixture-encrypted-secret"),
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := recovery.Plan(bundle, key)
	if err != nil || !plan.Verified || !plan.DatabaseValid || !plan.DesiredStateValid || !plan.ComposeValid || !plan.EncryptedSecrets || len(plan.Appdata) != 1 {
		t.Fatalf("fixture recovery plan is incomplete: %#v, %v", plan, err)
	}
}
