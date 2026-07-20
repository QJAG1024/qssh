//go:build !linux && !windows

package internal

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// processStartTime returns the best-effort process start time on non-Linux
// Unix systems. A reliable cross-Unix start-time is hard to obtain without
// cgo / libproc, so we treat it as unavailable. This causes callers to fall
// back to executable-path matching for PID identity.
func processStartTime(pid int) (uint64, error) {
	_ = pid
	return 0, fmt.Errorf("process start time unavailable on this platform")
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
