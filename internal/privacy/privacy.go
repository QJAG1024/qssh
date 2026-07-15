// Package privacy redacts host/IP addresses from user-visible output.
//
// Default is ON. Users can set a sticky override that lasts until reboot
// (runtime dir file), or pass --reveal for a single process.
package privacy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	// Placeholder shown when a host/IP is redacted.
	Redacted = "***"
	// envKey allows scripts to force mode without writing sticky state.
	envKey = "QSSH_PRIVACY"
)

// sticky file contents
const (
	stickyOn  = "on"
	stickyOff = "off"
)

var (
	mu sync.RWMutex
	// revealOnce forces privacy off for this process only.
	revealOnce bool
	// stickyCache is loaded once from disk/env (process lifetime).
	stickyLoaded bool
	stickyValue  *bool // nil = no sticky override
)

// Enabled reports whether host/IP redaction is active for this process.
//
// Priority: --reveal (process) > QSSH_PRIVACY env > runtime sticky > default ON.
func Enabled() bool {
	mu.RLock()
	if revealOnce {
		mu.RUnlock()
		return false
	}
	mu.RUnlock()

	if v, ok := envOverride(); ok {
		return v
	}

	mu.Lock()
	defer mu.Unlock()
	if !stickyLoaded {
		stickyValue = readSticky()
		stickyLoaded = true
	}
	if stickyValue != nil {
		return *stickyValue
	}
	return true // default ON
}

// RevealOnce disables privacy for the remainder of this process only.
// Does not write sticky state.
func RevealOnce() {
	mu.Lock()
	revealOnce = true
	mu.Unlock()
}

// SetSticky persists on/off until reboot (runtime dir). Empty clears sticky.
func SetSticky(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	path, err := stickyPath()
	if err != nil {
		return err
	}
	switch mode {
	case "", "default", "clear", "reset":
		_ = os.Remove(path)
		mu.Lock()
		stickyLoaded = true
		stickyValue = nil
		mu.Unlock()
		return nil
	case "on", "true", "1", "yes":
		if err := writeSticky(path, stickyOn); err != nil {
			return err
		}
		on := true
		mu.Lock()
		stickyLoaded = true
		stickyValue = &on
		mu.Unlock()
		return nil
	case "off", "false", "0", "no":
		if err := writeSticky(path, stickyOff); err != nil {
			return err
		}
		off := false
		mu.Lock()
		stickyLoaded = true
		stickyValue = &off
		mu.Unlock()
		return nil
	default:
		return fmt.Errorf("invalid privacy mode %q (want on|off|clear)", mode)
	}
}

// Status describes the effective mode and where it came from.
func Status() (enabled bool, source string) {
	mu.RLock()
	if revealOnce {
		mu.RUnlock()
		return false, "reveal"
	}
	mu.RUnlock()

	if v, ok := envOverride(); ok {
		if v {
			return true, "env"
		}
		return false, "env"
	}

	mu.Lock()
	defer mu.Unlock()
	if !stickyLoaded {
		stickyValue = readSticky()
		stickyLoaded = true
	}
	if stickyValue != nil {
		if *stickyValue {
			return true, "sticky"
		}
		return false, "sticky"
	}
	return true, "default"
}

// Host redacts a hostname or IP for display.
func Host(s string) string {
	if s == "" || !Enabled() {
		return s
	}
	return Redacted
}

// Addr redacts host:port or bare host for display.
func Addr(s string) string {
	if s == "" || !Enabled() {
		return s
	}
	// Preserve structure vaguely without leaking the host part.
	if host, port, ok := splitHostPort(s); ok {
		_ = host
		return Redacted + ":" + port
	}
	return Redacted
}

// UserAt formats user@host:port with redaction when enabled.
func UserAt(user, host string, port int) string {
	if !Enabled() {
		return fmt.Sprintf("%s@%s:%d", user, host, port)
	}
	return fmt.Sprintf("%s@%s", user, Redacted)
}

// Error returns err.Error() with host/IP tokens scrubbed when privacy is on.
// Use for any user-facing error print that may embed net.OpError addresses.
func Error(err error) string {
	if err == nil {
		return ""
	}
	return Scrub(err.Error())
}

// Scrub redacts IPv4/IPv6 and common "lookup host:" / "dial tcp host:port" forms.
// When privacy is off, returns s unchanged.
func Scrub(s string) string {
	if s == "" || !Enabled() {
		return s
	}
	out := s
	// dial tcp 1.2.3.4:22 / tcp/[::1]:22
	out = ipv4PortRe.ReplaceAllString(out, Redacted+":$1")
	out = ipv4Re.ReplaceAllString(out, Redacted)
	out = ipv6BracketPortRe.ReplaceAllString(out, Redacted+":$1")
	out = ipv6BracketRe.ReplaceAllString(out, Redacted)
	// "lookup example.com: " / "lookup 1.2.3.4: "
	out = lookupRe.ReplaceAllString(out, "lookup "+Redacted+":")
	// dial ... host:port remaining (hostname:digits) after IP scrub
	out = hostPortRe.ReplaceAllString(out, Redacted+":$1")
	return out
}

// --- internals ---

var (
	// 1.2.3.4:22 (port captured)
	ipv4PortRe = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}:(\d+)\b`)
	ipv4Re     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// [::1]:22
	ipv6BracketPortRe = regexp.MustCompile(`\[[0-9a-fA-F:]+\]:(\d+)`)
	ipv6BracketRe     = regexp.MustCompile(`\[[0-9a-fA-F:]+\]`)
	// lookup hostname: no such host
	lookupRe = regexp.MustCompile(`\blookup\s+([^:\s]+):`)
	// conservative hostname:port (after IPs scrubbed); require a letter so we
	// don't eat bare numbers.
	hostPortRe = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9._-]*:(\d+)\b`)
)

func envOverride() (enabled bool, ok bool) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envKey)))
	switch v {
	case "on", "true", "1", "yes":
		return true, true
	case "off", "false", "0", "no":
		return false, true
	default:
		return false, false
	}
}

func stickyPath() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		// Fallback that still dies on reboot on most systems.
		base = filepath.Join(os.TempDir(), fmt.Sprintf("qssh-privacy-%d", os.Getuid()))
		if err := os.MkdirAll(base, 0700); err != nil {
			return "", err
		}
		return filepath.Join(base, "mode"), nil
	}
	dir := filepath.Join(base, "qssh")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "privacy"), nil
}

func readSticky() *bool {
	path, err := stickyPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(string(data))) {
	case stickyOn:
		v := true
		return &v
	case stickyOff:
		v := false
		return &v
	default:
		return nil
	}
}

func writeSticky(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value+"\n"), 0600)
}

// splitHostPort is a tiny helper that avoids net.SplitHostPort's IPv6 bracket rules
// for display purposes only.
func splitHostPort(s string) (host, port string, ok bool) {
	// [ipv6]:port
	if strings.HasPrefix(s, "[") {
		i := strings.LastIndex(s, "]:")
		if i < 0 {
			return "", "", false
		}
		return s[1:i], s[i+2:], true
	}
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	// Avoid treating bare IPv6 as host:port (multiple colons, no brackets).
	if strings.Count(s, ":") > 1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
