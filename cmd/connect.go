package cmd

import (
	"fmt"
	"os"
	"time"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sshclient"
	"qssh/store"
)

// Connect establishes an SSH connection to the named profile.
func Connect(name string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error"), err)
		os.Exit(1)
	}

	p, exists := s.Get(name)
	if !exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
		os.Exit(1)
	}

	internal.RenderProfileHeader(p.Name, p.User, p.Host, p.Port)

	session, err := dialProfile(p, s)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("connect.failed"))
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	startTime := time.Now()
	if err := session.InteractiveShell(os.Stdin, os.Stdout, os.Stderr, internal.RenderProgress); err != nil {
		fmt.Fprintf(os.Stderr, "\n"+i18n.T("connect.ended")+"\n", err)
	}

	duration := time.Since(startTime)
	internal.RenderSummary(p.Name, formatDuration(duration))

	s.Touch(name)
	internal.AppendHistory(&internal.HistoryEntry{
		Profile:  p.Name,
		Duration: formatDuration(duration),
		Command:  "",
		ExitCode: 0,
	})
}

// dialProfile connects to a profile, resolving any proxy chain via the store.
func dialProfile(p store.Profile, st *store.Store) (*sshclient.Session, error) {
	return sshclient.DialProfile(p, st.Get, internal.RenderProgress)
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
