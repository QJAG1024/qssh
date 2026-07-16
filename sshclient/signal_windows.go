//go:build windows

package sshclient

import "syscall"

// windowChangeSignals returns the signals to listen for terminal resize.
// SIGWINCH is not available on Windows.
func windowChangeSignals() []syscall.Signal {
	return nil
}

// onWindowChange handles terminal resize. On Windows this is a no-op
// since SIGWINCH is not available. Signature is (height, width).
func onWindowChange(rawFd int, resizeFn func(h, w int)) {
	// No-op on Windows
}