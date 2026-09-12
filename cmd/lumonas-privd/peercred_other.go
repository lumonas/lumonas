//go:build !linux

package main

import "net"

// Non-Linux development hosts do not expose Linux SO_PEERCRED. The socket is
// still protected by its mode; Linux appliance builds use peercred_linux.go.
func peerAllowed(_ net.Conn, _ int) bool { return true }
