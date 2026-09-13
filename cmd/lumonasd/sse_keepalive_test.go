package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamSendsKeepaliveComments(t *testing.T) {
	server := testServer(t)
	previousInterval := sseHeartbeatInterval
	sseHeartbeatInterval = 5 * time.Millisecond
	defer func() { sseHeartbeatInterval = previousInterval }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	writer := &synchronizedResponseWriter{header: make(http.Header)}
	done := make(chan struct{})
	go func() {
		server.stream(writer, request)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(writer.snapshot(), ": keep-alive\n\n") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE stream did not stop after request cancellation")
	}
	if !strings.Contains(writer.snapshot(), ": keep-alive\n\n") {
		t.Fatal("SSE stream did not send a keepalive comment")
	}
}
