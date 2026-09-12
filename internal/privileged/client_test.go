package privileged

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/trace"
)

type pipeDialer struct{ connection net.Conn }

func (d pipeDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return d.connection, nil
}

func TestClientRoundTrip(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	go func() {
		defer server.Close()
		var request Request
		if err := json.NewDecoder(server).Decode(&request); err != nil {
			return
		}
		if request.CorrelationID != "corr-test" {
			return
		}
		_ = json.NewEncoder(server).Encode(Response{OK: request.Confirmed, Data: map[string]string{"status": "accepted"}})
	}()
	ctx := trace.WithCorrelationID(context.Background(), "corr-test")
	response, err := (Client{Dialer: pipeDialer{connection: client}}).Execute(ctx, Request{Operation: "filesystem.mount", PlanHash: "hash", Confirmed: true})
	if err != nil || !response.OK {
		t.Fatalf("unexpected response: %#v err=%v", response, err)
	}
}

func TestClientDeadlineInterruptsPendingResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	requestRead := make(chan struct{})
	go func() {
		var request Request
		if err := json.NewDecoder(server).Decode(&request); err == nil {
			close(requestRead)
		}
		<-requestRead
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (Client{Dialer: pipeDialer{connection: client}}).Execute(ctx, Request{Operation: "filesystem.mount", PlanHash: "hash"})
		done <- err
	}()
	select {
	case <-requestRead:
	case <-time.After(time.Second):
		t.Fatal("privileged request was not written")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected pending privileged response to fail at context deadline")
		}
	case <-time.After(time.Second):
		t.Fatal("privileged client ignored context deadline")
	}
}
