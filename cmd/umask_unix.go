//go:build unix

package cmd

import "golang.org/x/sys/unix"

// umask sets the process umask and returns the previous value.
// Used to close the race where a Unix control socket is briefly
// world-accessible between bind and chmod.
func umask(mask int) int {
	return unix.Umask(mask)
}
