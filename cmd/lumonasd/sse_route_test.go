package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEventStreamCompatibilityRoutes(t *testing.T) {
	for _, path := range []string{"/api/v1/events", "/api/v1/events/stream"} {
		t.Run(path, func(t *testing.T) {
			server := testServer(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
			writer := &synchronizedResponseWriter{header: make(http.Header)}
			done := make(chan struct{})
			go func() {
				server.routes().ServeHTTP(writer, request)
				close(done)
			}()

			deadline := time.Now().Add(time.Second)
			for !strings.Contains(writer.snapshot(), "retry: 3000") && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("event stream route did not stop after request cancellation")
			}
			if !strings.Contains(writer.snapshot(), "retry: 3000") {
				t.Fatalf("event stream route did not emit the SSE retry frame: %q", writer.snapshot())
			}
		})
	}
}
