// Package testhttp provides network test helpers that avoid depending on the
// host's IPv6 loopback configuration.
package testhttp

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
)

// NewServer starts an httptest server on an IPv4 loopback listener. Some
// sandboxed macOS environments reject the default httptest [::1] listener;
// production services remain unchanged.
func NewServer(t testing.TB, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("loopback listener unavailable in this test environment: %v", err)
		}
		t.Fatalf("listen on IPv4 loopback: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}
