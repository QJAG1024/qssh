package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"qssh/internal/i18n"
	"qssh/store"
)

// List displays all profiles in a formatted table, optionally filtered.
func List(filter string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
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
			fmt.Printf(i18n.T("profile.list_empty_filter")+"\n", filter)
		} else {
			fmt.Println(i18n.T("profile.list_empty"))
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, i18n.T("list.header.name")+"\t"+i18n.T("list.header.host")+"\t"+i18n.T("list.header.port")+"\t"+i18n.T("list.header.user")+"\t"+i18n.T("list.header.auth")+"\t"+i18n.T("list.header.last_used")+"\t"+i18n.T("list.header.count"))
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