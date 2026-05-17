package internal

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
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
	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
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
	StepDecrypt       StepID = iota
	StepDNSResolve
	StepTCPConnect
	StepSSHHandshake // version + key exchange + host key verify
	StepAuthenticate
	StepAllocatePTY
	StepShellStart
)

func (s StepID) String() string {
	switch s {
	case StepDecrypt:
		return "凭据解密"
	case StepDNSResolve:
		return "DNS 解析"
	case StepTCPConnect:
		return "TCP 连接建立"
	case StepSSHHandshake:
		return "SSH 握手"
	case StepAuthenticate:
		return "认证"
	case StepAllocatePTY:
		return "PTY 分配"
	case StepShellStart:
		return "启动 Shell"
	default:
		return "未知步骤"
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
		msg = r.ID.String()
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
func RenderProfileHeader(name string, user string, host string, port int) {
	fmt.Fprintf(os.Stderr, "配置: %s (%s@%s:%d)\n", name, user, host, port)
}

// RenderSummary prints a brief connection end summary.
func RenderSummary(name string, duration string) {
	fmt.Fprintf(os.Stderr, "  ⚡ 连接已关闭 (%s)\n", duration)
}