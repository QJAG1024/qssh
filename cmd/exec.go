package cmd

import (
	"fmt"
	"os"
	"time"

	"qssh/internal"
)

// Exec connects to a profile, runs a command, and exits with the remote exit code.
// If a daemon is already running, reuses its connection.
// Otherwise, auto-starts a managed daemon (idle timeout 5 min, auto-exit).
func Exec(name, command string) {
	start := time.Now()
	var code int
	var err error

	if daemonRunning(name) {
		code, err = execViaDaemon(name, command)
	} else {
		// Auto-start a managed daemon.
		if err := startManagedDaemon(name); err != nil {
			fmt.Fprintf(os.Stderr, "exec: %v\n", err)
			os.Exit(1)
		}
		code, err = execViaDaemon(name, command)
	}

	duration := time.Since(start)
	internal.AppendHistory(&internal.HistoryEntry{
		Profile:  name,
		Duration: duration.Truncate(time.Second).String(),
		Command:  command,
		ExitCode: code,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "exec via daemon: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}
