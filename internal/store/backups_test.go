package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
)

func TestBackupDestinationsEncryptCredentialsAndPersistRuns(t *testing.T) {
	database, err := Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	destination, err := database.SaveBackupDestination(backup.Destination{ID: "local", Name: "Local", Type: backup.DestinationLocal, Target: "/var/lib/mynas/recovery", Enabled: true, Retention: backup.DefaultRetention()}, backup.Credentials{AccessKey: "access", SecretKey: "secret"}, []byte("recovery-key"))
	if err != nil || !destination.CredentialsConfigured {
		t.Fatalf("save destination failed: %#v %v", destination, err)
	}
	values, err := database.ListBackupDestinations()
	if err != nil || len(values) != 1 || !values[0].CredentialsConfigured {
		t.Fatalf("unexpected destinations: %#v %v", values, err)
	}
	loaded, credentials, err := database.BackupDestination("local", []byte("recovery-key"))
	if err != nil || loaded.ID != "local" || credentials.SecretKey != "secret" {
		t.Fatalf("credential load failed: %#v %#v %v", loaded, credentials, err)
	}
	run := backup.Run{ID: "run-1", Trigger: "manual", Generation: 7, State: "verified", StartedAt: time.Now().UTC()}
	if err := database.SaveBackupRun(run); err != nil {
		t.Fatal(err)
	}
	copy := backup.Copy{ID: "copy-1", RunID: run.ID, DestinationID: destination.ID, Object: "recovery/generation-7.mrb", Checksum: "abc", Bytes: 3, State: "verified", Verified: true, CreatedAt: time.Now().UTC()}
	if err := database.SaveBackupCopy(copy); err != nil {
		t.Fatal(err)
	}
	copies, err := database.BackupCopies(run.ID)
	if err != nil || len(copies) != 1 || !copies[0].Verified {
		t.Fatalf("unexpected copies: %#v %v", copies, err)
	}
}
