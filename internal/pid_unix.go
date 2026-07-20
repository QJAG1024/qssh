//go:build !linux && !windows

package internal

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// processStartTime returns the best-effort process start time on non-Linux
// Unix systems. macOS/BSD provide a usable lstart via ps, so we parse that.
// The returned value is Unix seconds, which is sufficient for identity checks
// because callers compare against the value they stored on the same platform.
func processStartTime(pid int) (uint64, error) {
	// Force the C locale so ps output is stable across systems.
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ps lstart: %w", err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, fmt.Errorf("ps lstart returned empty")
	}
	// macOS/BSD lstart output under LC_ALL=C looks like:
	// "Sun Jan  2 15:04:05 2006" (day may be padded or not).
	tm, err := time.Parse("Mon Jan _2 15:04:05 2006", s)
	if err != nil {
		return 0, fmt.Errorf("parse lstart %q: %w", s, err)
	}
	return uint64(tm.Unix()), nil
}

// processExe returns the executable path for a process on non-Linux Unix
// systems. It uses lsof and looks for the text (code) file descriptor, which
// gives the full binary path on macOS/BSD. If lsof fails, it falls back to
// ps comm= (basename only).
func processExe(pid int) (string, error) {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid), "-Fn").Output()
	if err == nil {
		// lsof -Fn output groups file descriptors. The executable is marked
		// with an "ftxt" line; the following "n" line is the full path.
		next := false
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if next && strings.HasPrefix(line, "n") {
				return strings.TrimPrefix(line, "n"), nil
			}
			if line == "ftxt" {
				next = true
			}
		}
	}
	out, err = exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return "", fmt.Errorf("ps comm: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
