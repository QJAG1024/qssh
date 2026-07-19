package cmd

import (
	"fmt"
	"os"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sftpproxy"
)

// Delete removes a profile after confirmation.
// When force is true (--yes/-y), skips the interactive prompt (agent-friendly).
// Stops any running daemon for the profile first.
func Delete(name string, force bool) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
		os.Exit(1)
	}

	if _, exists := s.Get(name); !exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
		os.Exit(1)
	}

	if !force {
		if !internal.Confirm(fmt.Sprintf(i18n.T("profile.delete_confirm"), name), false) {
			fmt.Println(i18n.T("profile.cancelled"))
			return
		}
	}

	// Revoke any running daemon — an authenticated session should not outlive
	// the profile that authorized it.
	if daemonRunning(name) {
		_ = stopDaemon(name)
	}
	// Clean up leftover socket/pid files whether daemon was running or not.
	_ = os.Remove(daemonSocketPath(name))
	_ = os.Remove(daemonPidPath(name))
	// Also stop SFTP if mounted for this profile.
	_ = sftpproxy.Stop(name)

	if err := s.Delete(name); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("profile.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf(i18n.T("profile.deleted")+"\n", name)
}
