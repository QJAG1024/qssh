package cmd

import (
	"fmt"
	"os"
	"time"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sshclient"
)

// Connect establishes an SSH connection to the named profile.
func Connect(name string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
		os.Exit(1)
	}

	p, exists := s.Get(name)
	if !exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
		os.Exit(1)
	}

	internal.RenderProfileHeader(p.Name, p.User, p.Host, p.Port)

	session, err := sshclient.Dial(p, internal.RenderProgress)
	if err != nil {
		// Dial already rendered failure steps via progress callback.
		fmt.Fprintln(os.Stderr, i18n.T("connect.failed"))
		os.Exit(1)
	}
	defer session.Close()

	startTime := time.Now()
	if err := session.InteractiveShell(os.Stdin, os.Stdout, os.Stderr, internal.RenderProgress); err != nil {
		// Session ended with error — still count it as connected.
		fmt.Fprintf(os.Stderr, "\n"+i18n.T("connect.ended")+"\n", err)
	}

	duration := time.Since(startTime)
	internal.RenderSummary(p.Name, formatDuration(duration))

	// Update profile stats.
	s.Touch(name)
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}