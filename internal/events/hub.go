package events

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/lumonas/lumonas/internal/model"
)

type Hub struct {
	mu      sync.Mutex
	nextID  uint64
	clients map[chan model.Event]struct{}
}

func NewHub() *Hub { return &Hub{clients: make(map[chan model.Event]struct{})} }

func (h *Hub) Subscribe() (<-chan model.Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan model.Event, 32)
	h.clients[ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Publish(event model.Event) {
	h.mu.Lock()
	h.nextID++
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt-%d", h.nextID)
	}
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
		}
	}
	h.mu.Unlock()
}

func Encode(event model.Event) ([]byte, error) { return json.Marshal(event) }
