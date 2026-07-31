package internal

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
	"qssh/internal/i18n"
	"qssh/internal/privacy"
)

// Shared buffered reader for stdin — avoids buffering issues with pipes.
var stdinReader = bufio.NewReader(os.Stdin)

// isTerminal returns true if stdin is a terminal (not piped).
var isTerminal = term.IsTerminal(int(os.Stdin.Fd()))

// readLine reads a line from the shared stdin reader.
func readLine() string {
	line, _ := stdinReader.ReadString('\n')
	return strings.TrimSpace(line)
}

// Prompt reads a line from stdin with an optional default value.
// Returns the input, or the default if input is empty.
func Prompt(label string, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	line := readLine()
	if line == "" {
		return defaultVal
	}
	return line
}

// ReadPassword reads a password without echoing to terminal.
func ReadPassword(label string) (string, error) {
	fmt.Printf("%s: ", label)
	var pass string
	var err error
	if isTerminal {
		raw, e := term.ReadPassword(int(os.Stdin.Fd()))
		err = e
		pass = string(raw)
	} else {
		// When piped, read from the shared reader (no echo hiding).
		pass, err = stdinReader.ReadString('\n')
		pass = strings.TrimSpace(pass)
	}
	fmt.Println()
	if err != nil {
		return "", err
	}
	return pass, nil
}

// Confirm prompts for a yes/no answer. Returns true if the user confirms.
func Confirm(label string, defaultYes bool) bool {
	suffix := i18n.T("prompt.confirm_no")
	if defaultYes {
		suffix = i18n.T("prompt.confirm_yes")
	}
	fmt.Printf("%s%s", label, suffix)

	line := strings.ToLower(readLine())
	switch line {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultYes
	}
}

// --- Progress reporting ---

// StepID identifies a step in the SSH connection process.
type StepID int

const (
	StepDecrypt StepID = iota
	StepDNSResolve
	StepTCPConnect
	StepSSHHandshake // version + key exchange + host key verify
	StepAuthenticate
	StepProxyConnect // connecting through a jump host
	StepAllocatePTY
	StepShellStart
)

func (s StepID) String() string {
	switch s {
	case StepDecrypt:
		return "decrypt"
	case StepDNSResolve:
		return "dns_resolve"
	case StepTCPConnect:
		return "tcp_connect"
	case StepSSHHandshake:
		return "ssh_handshake"
	case StepProxyConnect:
		return "proxy_connect"
	case StepAuthenticate:
		return "authenticate"
	case StepAllocatePTY:
		return "allocate_pty"
	case StepShellStart:
		return "shell_start"
	default:
		return "unknown"
	}
}

// StepStatus represents the state of a connection step.
type StepStatus int

const (
	StepRunning StepStatus = iota
	StepDone
	StepFailed
	StepSkipped
)

func (s StepStatus) String() string {
	switch s {
	case StepRunning:
		return "→"
	case StepDone:
		return "✔"
	case StepFailed:
		return "✘"
	case StepSkipped:
		return "−"
	default:
		return "?"
	}
}

// StepResult is reported for each step in the connection process.
type StepResult struct {
	ID      StepID
	Status  StepStatus
	Message string // Brief step description
	Detail  string // Optional: timing, algorithm name, etc.
	Hint    string // Optional: troubleshooting hint on failure
}

// ProgressFn is a callback for reporting connection progress.
type ProgressFn func(StepResult)

// NopProgress is a no-op progress reporter (for testing).
func NopProgress(StepResult) {}

// --- Formatted output ---

// RenderProgress prints a single progress step to stderr (visible but not captured).
func RenderProgress(r StepResult) {
	status := r.Status.String()
	msg := r.Message
	if msg == "" {
		msg = i18n.T("step." + r.ID.String())
	}
	line := fmt.Sprintf("  %s %s", status, msg)
	if r.Detail != "" {
		line += fmt.Sprintf(" (%s)", r.Detail)
	}
	fmt.Fprint(os.Stderr, line, "\r\n")

	if r.Status == StepFailed && r.Hint != "" {
		fmt.Fprintf(os.Stderr, "     ↑ %s\r\n", r.Hint)
	}
}

// RenderProfileHeader prints the connection header with profile info.
// Host is redacted when privacy mode is on.
func RenderProfileHeader(name string, user string, host string, port int) {
	if privacy.Enabled() {
		fmt.Fprintf(os.Stderr, i18n.T("profile.header_private")+"\n", name, privacy.UserAt(user, host, port))
		return
	}
	fmt.Fprintf(os.Stderr, i18n.T("profile.header")+"\n", name, user, host, port)
}

// RenderSummary prints a brief connection end summary.
func RenderSummary(name string, duration string) {
	fmt.Fprintf(os.Stderr, i18n.T("session.closed")+"\n", duration)
}

// --- Interactive prompt helpers ---

// SelectPrompt displays a numbered list and returns the chosen value.
// If the user presses Enter without a number, returns the default (the first
// item when defaultVal is empty, or the matching item otherwise).
func SelectPrompt(label string, items []string, defaultVal string) string {
	fmt.Printf("%s\n", label)
	for i, item := range items {
		fmt.Printf("  %d) %s\n", i+1, item)
	}
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", i18n.T("prompt.select"), defaultVal)
	} else {
		fmt.Printf("%s [1]: ", i18n.T("prompt.select"))
	}
	line := readLine()
	if line == "" {
		if defaultVal != "" {
			return defaultVal
		}
		return items[0]
	}
	// Try numeric
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(items) {
		return items[n-1]
	}
	// Try substring match
	lower := strings.ToLower(line)
	for _, item := range items {
		if strings.ToLower(item) == lower {
			return item
		}
	}
	// Fallback to first item
	return items[0]
}

// ReadPasswordWithConfirm reads a password twice and returns it only if both
// entries match.
func ReadPasswordWithConfirm(label string) (string, error) {
	pass, err := ReadPassword(label)
	if err != nil {
		return "", err
	}
	pass2, err := ReadPassword(label + " " + i18n.T("password.confirm_suffix"))
	if err != nil {
		return "", err
	}
	if pass != pass2 {
		return "", errors.New(i18n.T("password.mismatch"))
	}
	return pass, nil
}

// ExpandPath expands ~ to the home directory.
func ExpandPath(path string) string {
	if len(path) > 1 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return home + path[1:]
		}
	}
	return path
}
