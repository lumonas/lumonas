package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestPruneEventsKeepsNewestWindow(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for index := 0; index < 110; index++ {
		if err := database.SaveEvent(model.Event{ID: "event-" + string(rune(index)), Type: "test", Timestamp: time.Now().UTC().Add(time.Duration(index) * time.Second), Severity: "info", Data: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.PruneEvents(100); err != nil {
		t.Fatal(err)
	}
	items, err := database.Events(500)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 100 {
		t.Fatalf("expected 100 retained events, got %d", len(items))
	}
}
