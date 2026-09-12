package privileged

import (
	"context"
	"encoding/json"
	"net"
	"testing"
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
		_ = json.NewEncoder(server).Encode(Response{OK: request.Confirmed, Data: map[string]string{"status": "accepted"}})
	}()
	response, err := (Client{Dialer: pipeDialer{connection: client}}).Execute(context.Background(), Request{Operation: "filesystem.mount", PlanHash: "hash", Confirmed: true})
	if err != nil || !response.OK {
		t.Fatalf("unexpected response: %#v err=%v", response, err)
	}
}
