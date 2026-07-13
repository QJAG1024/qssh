//go:build !linux

package cmd

import "net"

// peerCred is unavailable on non-Linux; socket mode 0600 is the access control.
func peerCred(conn net.Conn) (pid int, uid int, err error) {
	return 0, 0, nil
}

// authorizePeer is a no-op on non-Linux (permissions on the socket file apply).
func authorizePeer(conn net.Conn) error {
	return nil
}

func peerPID(conn net.Conn) (int, error) {
	return 0, nil
}
