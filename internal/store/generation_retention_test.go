package store

import (
	"testing"
	"time"
)

func TestPruneConfigGenerationsKeepsPendingAndNewestCommitted(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(timeFormat)
	for generation := int64(2); generation <= 106; generation++ {
		if _, err := database.db.Exec(`INSERT INTO config_generations(generation,state,plan_hash,created_at,committed_at) VALUES(?,?,?,?,?)`, generation, "committed", "hash", now, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.db.Exec(`INSERT INTO config_generations(generation,state,plan_hash,created_at) VALUES(?,?,?,?)`, 1000, "pending", "pending-hash", now); err != nil {
		t.Fatal(err)
	}

	if err := database.PruneConfigGenerations(100); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database, "config_generations", 101)
	assertGenerationState(t, database, 106, "committed")
	assertGenerationState(t, database, 1000, "pending")
	var oldest int64
	if err := database.db.QueryRow(`SELECT MIN(generation) FROM config_generations WHERE state='committed'`).Scan(&oldest); err != nil {
		t.Fatal(err)
	}
	if oldest != 7 {
		t.Fatalf("expected newest 100 committed generations to start at 7, got %d", oldest)
	}
}

func assertGenerationState(t *testing.T, database *Store, generation int64, want string) {
	t.Helper()
	var state string
	if err := database.db.QueryRow(`SELECT state FROM config_generations WHERE generation=?`, generation).Scan(&state); err != nil {
		t.Fatalf("generation %d was pruned: %v", generation, err)
	}
	if state != want {
		t.Fatalf("generation %d: expected state %q, got %q", generation, want, state)
	}
}
