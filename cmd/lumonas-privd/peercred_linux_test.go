//go:build linux

package main

import (
	"net"
	"os"
	"testing"
)

func TestPeerAllowedRejectsNonUnixConnections(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	if peerAllowed(left, 0) {
		t.Fatal("non-Unix connections must not pass peer credential checks")
	}
}

func TestPeerAllowedAcceptsCurrentUnixPeer(t *testing.T) {
	socketPath := t.TempDir() + "/privd.sock"
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
			return
		}
		close(accepted)
	}()
	client, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, ok := <-accepted
	if !ok {
		t.Fatal("listener failed to accept Unix peer")
	}
	defer server.Close()
	if !peerAllowed(server, os.Getgid()) {
		t.Fatal("current Unix peer should pass its service-group check")
	}
}
