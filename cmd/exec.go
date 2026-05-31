package cmd

import (
	"fmt"
	"os"
)

// Exec connects to a profile, runs a command, and exits with the remote exit code.
// If a daemon is already running, reuses its connection.
// Otherwise, auto-starts a managed daemon (idle timeout 5 min, auto-exit).
func Exec(name, command string) {
	if daemonRunning(name) {
		code, err := execViaDaemon(name, command)
		if err != nil {
			fmt.Fprintf(os.Stderr, "exec via daemon: %v\n", err)
			os.Exit(1)
		}
		os.Exit(code)
	}

	// Auto-start a managed daemon.
	if err := startManagedDaemon(name); err != nil {
		fmt.Fprintf(os.Stderr, "exec: %v\n", err)
		os.Exit(1)
	}

	code, err := execViaDaemon(name, command)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec via daemon: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}
