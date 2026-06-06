package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"qssh/internal"
	"qssh/internal/i18n"
)

// History displays connection history.
func History(profile string, lastOnly bool) {
	entries, err := internal.ReadHistory()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading history: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		if profile != "" {
			fmt.Println(i18n.T("history.empty"))
		} else {
			fmt.Println(i18n.T("history.empty_all"))
		}
		return
	}

	// Filter and reverse-sort by time (newest first).
	filtered := make([]internal.HistoryEntry, 0, len(entries))
	for _, e := range entries {
		if profile == "" || e.Profile == profile {
			filtered = append(filtered, e)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	if lastOnly && len(filtered) > 0 {
		filtered = filtered[:1]
	}

	// Limit to latest 50 if showing all.
	if !lastOnly && len(filtered) > 50 {
		filtered = filtered[:50]
	}

	if profile != "" {
		fmt.Printf(i18n.T("history.header")+"\n", profile)
	} else {
		fmt.Println(i18n.T("history.header_all"))
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, i18n.T("history.time")+"\t"+i18n.T("history.duration")+"\t"+i18n.T("history.command")+"\t"+i18n.T("history.exit"))
	fmt.Fprintln(w, "----\t--------\t-------\t---------")

	for _, e := range filtered {
		ts := e.Timestamp.Format("15:04:05 01-02")
		cmd := e.Command
		if cmd == "" {
			cmd = "<shell>"
		}
		dur := e.Duration
		if dur == "" && e.DurationMs > 0 {
			dur = fmt.Sprintf("%ds", e.DurationMs/1000)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", ts, dur, cmd, e.ExitCode)
	}
	w.Flush()
}