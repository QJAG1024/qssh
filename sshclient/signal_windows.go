//go:build windows

package sshclient

import "syscall"

// windowChangeSignals returns the signals to listen for terminal resize.
// SIGWINCH is not available on Windows.
func windowChangeSignals() []syscall.Signal {
	return nil
}

// onWindowChange handles terminal resize. On Windows this is a no-op
// since SIGWINCH is not available.
func onWindowChange(rawFd int, resizeFn func(w, h int)) {
	// No-op on Windows
}