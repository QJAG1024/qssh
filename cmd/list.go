package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"qssh/store"
)

// List displays all profiles in a formatted table, optionally filtered.
func List(filter string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}

	var profiles []store.Profile
	if filter != "" {
		profiles = s.Search(filter)
	} else {
		names := s.List()
		all := s.GetAll()
		profiles = make([]store.Profile, 0, len(names))
		for _, n := range names {
			if p, ok := all[n]; ok {
				profiles = append(profiles, p)
			}
		}
	}

	if len(profiles) == 0 {
		if filter != "" {
			fmt.Printf("No profiles matching %q.\n", filter)
		} else {
			fmt.Println("No profiles. Use 'qssh --add <name>' to create one.")
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Name\tHost\tPort\tUser\tAuth\tLast Used\tCount")
	fmt.Fprintln(w, "----\t----\t----\t----\t----\t---------\t-----")

	for _, p := range profiles {
		lastUsed := "-"
		if !p.LastUsed.IsZero() {
			lastUsed = formatTimeAgo(p.LastUsed)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%d\n",
			p.Name, p.Host, p.Port, p.User, p.Auth, lastUsed, p.ConnectionCount)
	}
	w.Flush()
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	default:
		return t.Format("Jan 2")
	}
}