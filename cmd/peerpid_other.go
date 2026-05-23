//go:build !linux

package cmd

import "net"

func peerPID(conn net.Conn) (int, error) {
	return 0, nil
}
