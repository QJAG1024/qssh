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
func onWindowChange(rawFd int, resizeFn func(w, h int)) {
	if rawFd >= 0 {
		w, h, _ := term.GetSize(rawFd)
		if w > 0 && h > 0 {
			resizeFn(h, w)
		}
	}
}