package cmd

import (
	"fmt"
	"io"
	"strings"

	"qssh/internal/i18n"
)

// usageCmd describes one top-level command in the help output.
// usage holds the command syntax (shared across locales, not translated);
// desc is an i18n key for the localized one-line description.
// subs lists sub-parameter flags that modify this command (optional).
type usageCmd struct {
	usage string // e.g. "qssh --add <name>"
	desc  string // i18n key
	subs  string // e.g. "host port user auth password key-path key-passphrase proxy set-option tags"
}

// usageGroup groups related commands under a localized section title.
type usageGroup struct {
	name string // i18n key for the group title
	cmds []usageCmd
}

// usageGroups is the single source of truth for the help output. Adding a
// top-level command means adding one entry here; sub-parameters are attached
// to their parent's subs field. Internal flags (--daemon-run, --bind-addr,
// etc.) are intentionally absent — they are not user-facing.
var usageGroups = []usageGroup{
	{
		name: "usage.group.create",
		cmds: []usageCmd{
			{
				usage: "qssh --add <name>",
				desc:  "usage.desc.add",
				subs:  "host port user auth password key-path key-passphrase proxy set-option tags",
			},
			{
				usage: "qssh --edit <name>",
				desc:  "usage.desc.edit",
				subs:  "host port user auth password key-path key-passphrase proxy set-option tags",
			},
			{
				usage: "qssh --copy <old> <new>",
				desc:  "usage.desc.copy",
			},
			{
				usage: "qssh --rename <old> <new>",
				desc:  "usage.desc.rename",
			},
			{
				usage: "qssh --delete <name>",
				desc:  "usage.desc.delete",
			},
		},
	},
	{
		name: "usage.group.connect",
		cmds: []usageCmd{
			{
				usage: "qssh <profile>",
				desc:  "usage.desc.connect",
			},
			{
				usage: "qssh --exec <profile> <command>",
				desc:  "usage.desc.exec",
			},
			{
				usage: "qssh --sftp-start <name>",
				desc:  "usage.desc.sftp_start",
				subs:  "bind port",
			},
			{
				usage: "qssh --sftp-stop <name>",
				desc:  "usage.desc.sftp_stop",
			},
		},
	},
	{
		name: "usage.group.manage",
		cmds: []usageCmd{
			{
				usage: "qssh --list [filter]",
				desc:  "usage.desc.list",
				subs:  "json",
			},
			{
				usage: "qssh --history [name]",
				desc:  "usage.desc.history",
				subs:  "last",
			},
			{
				usage: "qssh --daemon-start <name>",
				desc:  "usage.desc.daemon_start",
			},
			{
				usage: "qssh --daemon-stop <name>",
				desc:  "usage.desc.daemon_stop",
			},
			{
				usage: "qssh --config [get|set ...]",
				desc:  "usage.desc.config",
			},
			{
				usage: "qssh --privacy [on|off|clear|status]",
				desc:  "usage.desc.privacy",
			},
			{
				usage: "qssh --version",
				desc:  "usage.desc.version",
			},
		},
	},
}

// RenderUsage prints the full help text (banner + grouped commands) to w.
func RenderUsage(w io.Writer, version string) {
	// Banner: version is injected inside T (Sprintf), keeping the format
	// string out of Fprintf so a stray %% in a translation cannot break it.
	fmt.Fprintf(w, "%s\n\n", i18n.T("usage.banner", version))
	fmt.Fprintln(w, i18n.T("usage.usage")+":")
	fmt.Fprintln(w)

	// Compute column width across all groups for alignment.
	maxWidth := 0
	for _, g := range usageGroups {
		for _, c := range g.cmds {
			if len(c.usage) > maxWidth {
				maxWidth = len(c.usage)
			}
		}
	}

	for _, g := range usageGroups {
		fmt.Fprintln(w, i18n.T(g.name)+":")
		for _, c := range g.cmds {
			pad := strings.Repeat(" ", maxWidth-len(c.usage))
			fmt.Fprintf(w, "  %s%s  %s\n", c.usage, pad, i18n.T(c.desc))
			if c.subs != "" {
				// Sub-parameter hint on the next line, aligned under the desc.
				indent := strings.Repeat(" ", maxWidth+4)
				fmt.Fprintf(w, "%s%s: %s\n", indent, i18n.T("usage.subs"), c.subs)
			}
		}
		fmt.Fprintln(w)
	}
}
