package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

type synchronizedResponseWriter struct {
	mu     sync.Mutex
	header http.Header
	body   bytes.Buffer
}

func (w *synchronizedResponseWriter) Header() http.Header { return w.header }

func (w *synchronizedResponseWriter) WriteHeader(int) {}

func (w *synchronizedResponseWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(data)
}

func (w *synchronizedResponseWriter) Flush() {}

func (w *synchronizedResponseWriter) snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func TestStreamReplaysEventsAfterLastEventID(t *testing.T) {
	server := testServer(t)
	for _, event := range []model.Event{
		{ID: "evt-before", Type: "old", Timestamp: time.Now().UTC(), Severity: "info", Data: map[string]any{}},
		{ID: "evt-after", Type: "new", Timestamp: time.Now().UTC().Add(time.Second), Severity: "info", CorrelationID: "corr-replay", OperationID: "op-replay", PlanHash: "plan-replay", Data: map[string]any{"value": "replayed"}},
	} {
		if err := server.store.SaveEvent(event); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	request.Header.Set("Last-Event-ID", "evt-before")
	writer := &synchronizedResponseWriter{header: make(http.Header)}
	done := make(chan struct{})
	go func() {
		server.stream(writer, request)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(writer.snapshot(), "id: evt-after") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE stream did not stop after request cancellation")
	}

	body := writer.snapshot()
	if !strings.Contains(body, "id: evt-after") || !strings.Contains(body, "replayed") {
		t.Fatalf("replayed event missing from SSE response: %s", body)
	}
	if strings.Contains(body, "id: evt-before") {
		t.Fatalf("SSE response replayed the cursor event itself: %s", body)
	}
	if !strings.Contains(body, `"correlationId":"corr-replay"`) || !strings.Contains(body, `"operationId":"op-replay"`) || !strings.Contains(body, `"planHash":"plan-replay"`) {
		t.Fatalf("SSE response lost operation tracing fields: %s", body)
	}
}
