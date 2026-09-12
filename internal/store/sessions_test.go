package store

import (
	"testing"
	"time"
)

func TestSessionsListsNonExpiredRevocableSessionsWithoutTokens(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureAdmin("admin", "a-long-development-password"); err != nil {
		t.Fatal(err)
	}
	first, _, err := database.CreateSession("admin", "a-long-development-password", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := database.CreateSession("admin", "a-long-development-password", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	items, err := database.Sessions()
	if err != nil || len(items) != 2 {
		t.Fatalf("unexpected sessions %#v err=%v", items, err)
	}
	for _, item := range items {
		if item.ID == first || item.ID == second {
			t.Fatalf("session inventory exposed a raw token: %#v", item)
		}
		if item.Username != "admin" || item.ExpiresAt.Before(time.Now()) {
			t.Fatalf("unexpected session metadata: %#v", item)
		}
	}
	if err := database.DeleteSessionByID(items[0].ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := database.Sessions()
	if err != nil || len(remaining) != 1 {
		t.Fatalf("session revocation failed: %#v err=%v", remaining, err)
	}
}
