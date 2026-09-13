package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	_ "github.com/mattn/go-sqlite3"
)

func TestBackupDestinationsEncryptCredentialsAndPersistRuns(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	destination, err := database.SaveBackupDestination(backup.Destination{ID: "local", Name: "Local", Type: backup.DestinationLocal, Target: "/var/lib/lumonas/recovery", Enabled: true, Retention: backup.DefaultRetention()}, backup.Credentials{AccessKey: "access", SecretKey: "secret"}, []byte("recovery-key"))
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
	run := backup.Run{ID: "run-1", Actor: "admin", Trigger: "manual", Generation: 7, State: "verified", StartedAt: time.Now().UTC()}
	if err := database.SaveBackupRun(run); err != nil {
		t.Fatal(err)
	}
	runs, err := database.BackupRuns(10)
	if err != nil || len(runs) != 1 || runs[0].Actor != run.Actor {
		t.Fatalf("backup run actor was not persisted: %#v %v", runs, err)
	}
	copy := backup.Copy{ID: "copy-1", RunID: run.ID, DestinationID: destination.ID, Object: "recovery/generation-7.mrb", Checksum: "abc", Bytes: 3, State: "verified", Verified: true, CreatedAt: time.Now().UTC()}
	if err := database.SaveBackupCopy(copy); err != nil {
		t.Fatal(err)
	}
	copies, err := database.BackupCopies(run.ID)
	if err != nil || len(copies) != 1 || !copies[0].Verified {
		t.Fatalf("unexpected copies: %#v %v", copies, err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	due := time.Now().UTC().Add(time.Hour)
	schedule, err := database.SaveBackupSchedule(backup.Schedule{ID: "default", Enabled: true, IntervalSeconds: 86400, LastStartedAt: &started, NextDueAt: &due})
	if err != nil || schedule.ID != "default" {
		t.Fatalf("save schedule failed: %#v %v", schedule, err)
	}
	loadedSchedule, err := database.BackupSchedule()
	if err != nil || loadedSchedule.IntervalSeconds != 86400 || loadedSchedule.NextDueAt == nil {
		t.Fatalf("unexpected schedule: %#v %v", loadedSchedule, err)
	}
	verifiedAt := time.Now().UTC()
	if err := database.SaveBackupVerification(backup.Verification{ID: "verification-1", RunID: run.ID, DestinationID: destination.ID, State: "verified", VerifiedAt: &verifiedAt}); err != nil {
		t.Fatal(err)
	}
	verifications, err := database.BackupVerifications(10)
	if err != nil || len(verifications) != 1 || verifications[0].State != "verified" {
		t.Fatalf("unexpected verifications: %#v %v", verifications, err)
	}
}

func TestBackupScheduleRejectsUnboundedIntervals(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.SaveBackupSchedule(backup.Schedule{ID: "default", Enabled: true, IntervalSeconds: 1}); err == nil {
		t.Fatal("expected schedule interval validation")
	}
}

func TestFailInterruptedBackupRunsClosesRunsAndCopies(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.SaveBackupDestination(backup.Destination{ID: "local", Name: "Local", Type: backup.DestinationLocal, Target: "/tmp/lumonas-backups", Enabled: true, Retention: backup.DefaultRetention()}, backup.Credentials{}, []byte("recovery-key")); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	for _, state := range []string{"queued", "running"} {
		run := backup.Run{ID: "run-" + state, Trigger: "scheduled", Generation: 2, State: state, StartedAt: started}
		if err := database.SaveBackupRun(run); err != nil {
			t.Fatal(err)
		}
		if state == "running" {
			if err := database.SaveBackupCopy(backup.Copy{ID: "copy-running", RunID: run.ID, DestinationID: "local", Object: "recovery/run.mrb", Checksum: "abc", State: "running", CreatedAt: started}); err != nil {
				t.Fatal(err)
			}
		}
	}
	count, err := database.FailInterruptedBackupRuns("daemon restarted before backup completed")
	if err != nil || count != 2 {
		t.Fatalf("unexpected interrupted backup count: %d %v", count, err)
	}
	runs, err := database.BackupRuns(10)
	if err != nil || len(runs) != 2 {
		t.Fatalf("could not read reconciled runs: %#v %v", runs, err)
	}
	for _, run := range runs {
		if run.State != "failed" || run.FinishedAt == nil || run.Error == "" {
			t.Fatalf("interrupted run was not failed closed: %#v", run)
		}
	}
	copies, err := database.BackupCopies("run-running")
	if err != nil || len(copies) != 1 || copies[0].State != "failed" || copies[0].Verified {
		t.Fatalf("interrupted copy was not failed closed: %#v %v", copies, err)
	}
}

func TestOpenMigratesLegacyBackupRunActorColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE backup_runs (
id TEXT PRIMARY KEY, trigger_name TEXT NOT NULL, generation INTEGER NOT NULL,
state TEXT NOT NULL, bundle_path TEXT, checksum TEXT, bytes INTEGER NOT NULL DEFAULT 0,
started_at TEXT NOT NULL, finished_at TEXT, error TEXT
)`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	run := backup.Run{ID: "legacy-run", Actor: "admin", Trigger: "manual", Generation: 1, State: "queued", StartedAt: time.Now().UTC()}
	if err := database.SaveBackupRun(run); err != nil {
		t.Fatal(err)
	}
	runs, err := database.BackupRuns(10)
	if err != nil || len(runs) != 1 || runs[0].Actor != run.Actor {
		t.Fatalf("legacy backup run actor migration failed: %#v %v", runs, err)
	}
}
