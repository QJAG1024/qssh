//go:build linux

package cmd

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// peerCred returns the connecting process credentials (pid, uid).
func peerCred(conn net.Conn) (pid int, uid int, err error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, 0, fmt.Errorf("not unix conn")
	}
	f, err := uc.File()
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cred, err := syscall.GetsockoptUcred(int(f.Fd()), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		return 0, 0, err
	}
	return int(cred.Pid), int(cred.Uid), nil
}

// authorizePeer rejects connections from other UIDs.
func authorizePeer(conn net.Conn) error {
	_, uid, err := peerCred(conn)
	if err != nil {
		return fmt.Errorf("peer credentials: %w", err)
	}
	if uid != os.Getuid() {
		return fmt.Errorf("peer uid %d != self uid %d", uid, os.Getuid())
	}
	return nil
}

func peerPID(conn net.Conn) (int, error) {
	pid, _, err := peerCred(conn)
	return pid, err
}
