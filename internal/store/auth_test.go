package store

import (
	"testing"
	"time"
)

func TestAdminSessionLifecycle(t *testing.T) {
	db, err := Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.EnsureAdmin("admin", "a-long-development-password"); err != nil {
		t.Fatal(err)
	}
	if !db.HasUsers() {
		t.Fatal("admin user was not created")
	}
	token, expires, err := db.CreateSession("admin", "a-long-development-password", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !expires.After(time.Now()) {
		t.Fatal("session should expire in the future")
	}
	if user, ok := db.SessionUser(token); !ok || user != "admin" {
		t.Fatalf("unexpected session user %q %v", user, ok)
	}
	if err := db.DeleteSession(token); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.SessionUser(token); ok {
		t.Fatal("deleted session still valid")
	}
}

func TestDatabaseBackupProducesReadableCopy(t *testing.T) {
	db, err := Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetMeta("backup-test", "ok"); err != nil {
		t.Fatal(err)
	}
	backup, err := db.BackupBytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(backup) < 100 {
		t.Fatalf("backup is unexpectedly small: %d", len(backup))
	}
}
