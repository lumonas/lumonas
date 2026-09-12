package store

import "testing"

func TestConfigurationGenerationsAdvanceAndCommit(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if database.CurrentGeneration() != 1 {
		t.Fatalf("expected bootstrap generation 1, got %d", database.CurrentGeneration())
	}
	generation, err := database.BeginGeneration("plan-hash")
	if err != nil || generation != 2 {
		t.Fatalf("unexpected pending generation: %d %v", generation, err)
	}
	if err := database.CommitGeneration(generation); err != nil {
		t.Fatal(err)
	}
	if database.CurrentGeneration() != 2 {
		t.Fatalf("expected committed generation 2, got %d", database.CurrentGeneration())
	}
}
