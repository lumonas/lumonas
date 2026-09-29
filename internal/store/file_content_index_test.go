package store

import (
	"testing"
	"time"
)

func TestFileContentIndexDistinguishesUnbuiltFromEmpty(t *testing.T) {
	database := testStore(t)
	count, indexedAt, err := database.FileContentIndexStatus("share-a")
	if err != nil || count != 0 || indexedAt != nil {
		t.Fatalf("unexpected unbuilt index status: count=%d at=%v err=%v", count, indexedAt, err)
	}
	now := time.Now().UTC()
	if err := database.ReplaceFileContentIndex("share-a", []FileContentDocument{}, now); err != nil {
		t.Fatal(err)
	}
	count, indexedAt, err = database.FileContentIndexStatus("share-a")
	if err != nil || count != 0 || indexedAt == nil || indexedAt.IsZero() {
		t.Fatalf("completed empty index was mistaken for an unbuilt index: count=%d at=%v err=%v", count, indexedAt, err)
	}
	results, err := database.SearchFileContent("share-a", "missing", 10)
	if err != nil || len(results) != 0 {
		t.Fatalf("empty indexed share should return zero results: %#v %v", results, err)
	}
}
