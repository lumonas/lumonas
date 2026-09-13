package events

import (
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestUnsubscribeClosesClientAndFuturePublishRemainsSafe(t *testing.T) {
	hub := NewHub()
	channel, unsubscribe := hub.Subscribe()
	unsubscribe()

	select {
	case _, ok := <-channel:
		if ok {
			t.Fatal("unsubscribed event channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("unsubscribed event channel did not close")
	}

	hub.Publish(model.Event{Type: "after-unsubscribe", Timestamp: time.Now().UTC(), Data: map[string]any{}})
}

func TestPublishClosesSlowSubscriberForReplay(t *testing.T) {
	hub := NewHub()
	channel, unsubscribe := hub.Subscribe()
	defer unsubscribe()
	for index := 0; index < 33; index++ {
		hub.Publish(model.Event{ID: "evt", Type: "queued", Timestamp: time.Now().UTC(), Data: map[string]any{"index": index}})
	}

	for event := range channel {
		if event.Type != "queued" {
			t.Fatalf("unexpected buffered event %#v", event)
		}
	}
}

func TestHubPublishesToSubscribers(t *testing.T) {
	hub := NewHub()
	channel, unsubscribe := hub.Subscribe()
	defer unsubscribe()
	hub.Publish(model.Event{Type: "test", Timestamp: time.Now()})
	select {
	case event := <-channel:
		if event.Type != "test" {
			t.Fatalf("unexpected event %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHubDefaultsSchemaVersion(t *testing.T) {
	hub := NewHub()
	channel, unsubscribe := hub.Subscribe()
	defer unsubscribe()
	hub.Publish(model.Event{Type: "test", Timestamp: time.Now().UTC(), Data: map[string]any{}})

	event := <-channel
	if event.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", event.SchemaVersion, SchemaVersion)
	}
}

func TestEncodeDefaultsSchemaVersion(t *testing.T) {
	encoded, err := Encode(model.Event{Type: "test", Timestamp: time.Now().UTC(), Data: map[string]any{}})
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	if !strings.Contains(string(encoded), `"schemaVersion":1`) {
		t.Fatalf("encoded event missing schema version: %s", encoded)
	}
}
