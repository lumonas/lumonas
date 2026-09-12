//go:build linux

package main

import (
	"net"
	"syscall"
)

// peerAllowed adds a kernel-enforced identity check to the Unix socket mode.
// Root-owned worker services are allowed by UID 0; the broker also accepts its
// configured service group so only lumonasd can submit requests as a service
// user. Filesystem permissions remain the first gate, this is defense in depth.
func peerAllowed(connection net.Conn, allowedGID int) bool {
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := unixConnection.SyscallConn()
	if err != nil {
		return false
	}
	var credentials *syscall.Ucred
	var credentialErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, credentialErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || credentialErr != nil || credentials == nil {
		return false
	}
	return credentials.Uid == 0 || int(credentials.Gid) == allowedGID
}
