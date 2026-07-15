package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"qssh/internal"
	"qssh/internal/privacy"
)

// Exec connects to a profile, runs a command, and exits with the remote exit code.
// If a daemon is already running, reuses its connection.
// Otherwise, auto-starts a managed daemon (idle timeout 5 min, auto-exit).
//
// Prefer args (raw argv) when available so spaces/quotes are preserved via
// remote shell quoting. command is the display/history string.
func Exec(name string, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: --exec requires a command")
		os.Exit(1)
	}
	command := strings.Join(args, " ")
	start := time.Now()
	var code int
	var err error

	if !daemonRunning(name) {
		// Auto-start a managed daemon.
		if err := startManagedDaemon(name); err != nil {
			fmt.Fprintf(os.Stderr, "exec: %s\n", privacy.Error(err))
			os.Exit(1)
		}
	}
	code, err = execViaDaemon(name, args)

	duration := time.Since(start)
	internal.AppendHistory(&internal.HistoryEntry{
		Profile:  name,
		Duration: duration.Truncate(time.Second).String(),
		Command:  command,
		ExitCode: code,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "exec via daemon: %s\n", privacy.Error(err))
		os.Exit(1)
	}
	os.Exit(code)
}
