package cmd

import (
	"fmt"
	"os"
	"strings"

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
	// the profile that authorized it. stopDaemon uses force and PID fallback.
	// If the daemon survives the stop (rare), its keepalive loop will detect
	// the missing profile within 30s and auto-terminate.
	if err := stopDaemon(name); err != nil && daemonRunning(name) {
		fmt.Fprintf(os.Stderr, "warning: could not stop daemon for %q (it will auto-terminate within 30s): %v\n", name, err)
	}
	// Clean up leftover socket/pid files whether daemon was running or not.
	_ = os.Remove(daemonSocketPath(name))
	_ = os.Remove(daemonPidPath(name))
	// Also stop SFTP if mounted for this profile.
	if err := sftpproxy.Stop(name); err != nil {
		// "not running" is fine; other errors are warnings (state may be stale).
		if !strings.Contains(err.Error(), "is not running") {
			fmt.Fprintf(os.Stderr, "warning: stop sftp for %q: %v\n", name, err)
		}
	}

	if err := s.Delete(name); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("profile.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf(i18n.T("profile.deleted")+"\n", name)
}
