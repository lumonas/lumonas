package main

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
)

func TestPrivilegedProtocolRejectsUnknownOperation(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	go serve(server)
	if _, err := client.Write([]byte(`{"operation":"shell.exec","planHash":"confirmed"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	var response response
	if err := json.NewDecoder(bufio.NewReader(client)).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == "" {
		t.Fatalf("expected rejected operation, got %#v", response)
	}
}
