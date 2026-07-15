package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"qssh/internal/i18n"
	"qssh/internal/privacy"
	"qssh/store"
)

// listJSON is a redacted profile view for machine-readable output.
// Secrets (password, key passphrase) are never included.
type listJSON struct {
	Name            string            `json:"name"`
	Host            string            `json:"host,omitempty"`
	Port            int               `json:"port"`
	User            string            `json:"user"`
	Auth            string            `json:"auth"`
	KeyPath         string            `json:"key_path,omitempty"`
	Proxy           string            `json:"proxy,omitempty"`
	Options         map[string]string `json:"options,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	LastUsed        *time.Time        `json:"last_used,omitempty"`
	ConnectionCount int               `json:"connection_count"`
}

// List displays all profiles in a formatted table, optionally filtered.
// When asJSON is true, prints a JSON array (secrets redacted).
func List(filter string, asJSON bool) {
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

	if asJSON {
		out := make([]listJSON, 0, len(profiles))
		hideHost := privacy.Enabled()
		for _, p := range profiles {
			item := listJSON{
				Name:            p.Name,
				Port:            p.Port,
				User:            p.User,
				Auth:            string(p.Auth),
				KeyPath:         p.KeyPath,
				Proxy:           p.Proxy,
				Options:         p.Options,
				Tags:            p.Tags,
				CreatedAt:       p.CreatedAt,
				UpdatedAt:       p.UpdatedAt,
				ConnectionCount: p.ConnectionCount,
			}
			if !hideHost {
				item.Host = p.Host
			}
			// When privacy is on, omit host (empty + omitempty).
			if !p.LastUsed.IsZero() {
				t := p.LastUsed
				item.LastUsed = &t
			}
			out = append(out, item)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
			os.Exit(1)
		}
		return
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
	hideHost := privacy.Enabled()
	if hideHost {
		// Privacy: hide Host column entirely (profile name is the identity).
		fmt.Fprintln(w, i18n.T("list.header.name")+"\t"+i18n.T("list.header.port")+"\t"+i18n.T("list.header.user")+"\t"+i18n.T("list.header.auth")+"\t"+i18n.T("list.header.last_used")+"\t"+i18n.T("list.header.count")+"\t"+i18n.T("list.header.proxy"))
		fmt.Fprintln(w, "----\t----\t----\t----\t---------\t-----\t-----")
	} else {
		fmt.Fprintln(w, i18n.T("list.header.name")+"\t"+i18n.T("list.header.host")+"\t"+i18n.T("list.header.port")+"\t"+i18n.T("list.header.user")+"\t"+i18n.T("list.header.auth")+"\t"+i18n.T("list.header.last_used")+"\t"+i18n.T("list.header.count")+"\t"+i18n.T("list.header.proxy"))
		fmt.Fprintln(w, "----\t----\t----\t----\t----\t---------\t-----\t-----")
	}

	for _, p := range profiles {
		lastUsed := "-"
		if !p.LastUsed.IsZero() {
			lastUsed = formatTimeAgo(p.LastUsed)
		}
		proxy := "-"
		if p.Proxy != "" {
			proxy = p.Proxy
		}
		if hideHost {
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%d\t%s\n",
				p.Name, p.Port, p.User, p.Auth, lastUsed, p.ConnectionCount, proxy)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%d\t%s\n",
				p.Name, p.Host, p.Port, p.User, p.Auth, lastUsed, p.ConnectionCount, proxy)
		}
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
