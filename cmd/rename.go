package cmd

import (
	"fmt"
	"os"

	"qssh/sftpproxy"
)

// Rename renames a profile.
// Stops any daemon/SFTP bound to the old name so leftover sockets are not
// left under a name that no longer exists in the store.
func Rename(oldName, newName string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}

	// Revoke under the old name before rename so a still-running daemon
	// cannot serve after the profile identity moves.
	if err := stopDaemon(oldName); err != nil && daemonRunning(oldName) {
		fmt.Fprintf(os.Stderr, "warning: could not stop daemon for %q: %v\n", oldName, err)
	}
	_ = os.Remove(daemonSocketPath(oldName))
	_ = os.Remove(daemonPidPath(oldName))
	_ = sftpproxy.Stop(oldName)

	if err := s.Rename(oldName, newName); err != nil {
		fmt.Fprintf(os.Stderr, "Error renaming profile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Profile %q renamed to %q.\n", oldName, newName)
}
