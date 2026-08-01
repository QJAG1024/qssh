package cmd

import (
	"strings"

	"qssh/store"
)

// Platform-independent command/argv helpers. Kept out of daemon.go (which is
// //go:build !windows) so command-injection-safety tests run on every platform.

// buildRemoteCommand prefers Args over legacy Cmd.
// Multiple args are shell-quoted (safe argv). A single arg is treated as a
// full shell command string so `qssh --exec h 'echo hi'` keeps working.
func buildRemoteCommand(req daemonReq) string {
	switch len(req.Args) {
	case 0:
		return req.Cmd
	case 1:
		return req.Args[0]
	default:
		return shellJoin(req.Args)
	}
}

// shellJoin quotes each argument for a POSIX-like remote shell.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps s in single quotes, escaping embedded quotes as '\”.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Fast path: no metacharacters that need quoting.
	safe := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '@' || c == '+' || c == ',' {
			continue
		}
		safe = false
		break
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// profileIdentityEqual compares only the connection-defining fields of two
// profiles (host, port, user, auth, credentials, and proxy link).
func profileIdentityEqual(a, b store.Profile) bool {
	return a.Host == b.Host && a.Port == b.Port && a.User == b.User &&
		a.Auth == b.Auth && a.Proxy == b.Proxy &&
		a.Password == b.Password && a.KeyPath == b.KeyPath &&
		a.KeyPassphrase == b.KeyPassphrase
}
