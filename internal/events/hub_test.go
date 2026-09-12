package events

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

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
