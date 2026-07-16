//go:build !windows

package sshclient

import (
	"syscall"

	"golang.org/x/term"
)

// windowChangeSignals returns the signals to listen for terminal resize.
// SIGWINCH is not available on Windows.
func windowChangeSignals() []syscall.Signal {
	return []syscall.Signal{syscall.SIGWINCH}
}

// onWindowChange handles terminal resize. On Unix we can get the new size.
// resizeFn receives (height, width) to match ssh.Session.WindowChange.
func onWindowChange(rawFd int, resizeFn func(h, w int)) {
	if rawFd < 0 {
		return
	}
	w, h, err := term.GetSize(rawFd)
	if err != nil || w <= 0 || h <= 0 {
		return
	}
	resizeFn(h, w)
}