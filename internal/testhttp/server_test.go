package testhttp

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewServerUsesIPv4Loopback(t *testing.T) {
	server := NewServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if !strings.HasPrefix(server.URL, "http://127.0.0.1:") {
		t.Fatalf("server URL is not IPv4 loopback: %s", server.URL)
	}
}
