package cmd

import (
	"fmt"
	"os"

	"qssh/internal"
	"qssh/sshclient"
)

// Exec connects to a profile, runs a command, and exits with the remote exit code.
// If a daemon is running for this profile, reuses its SSH connection.
func Exec(name, command string) {
	// Try daemon first.
	if daemonRunning(name) {
		code, err := execViaDaemon(name, command)
		if err != nil {
			fmt.Fprintf(os.Stderr, "exec via daemon: %v\n", err)
			os.Exit(1)
		}
		os.Exit(code)
	}

	// Fall back to direct SSH connection.
	store, err := openStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p, exists := store.Get(name)
	if !exists {
		fmt.Fprintf(os.Stderr, "profile %q not found\n", name)
		os.Exit(1)
	}

	session, err := sshclient.Dial(p, internal.NopProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	code, err := session.RunCommand(command, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}