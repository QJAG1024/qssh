//go:build !linux && !windows

package internal

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// processStartTime returns the best-effort process start time on non-Linux
// Unix systems. It uses "ps -o lstart=" and parses the locale-independent
// format produced by macOS/BSD ps.
func processStartTime(pid int) (uint64, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
	if err != nil {
		return 0, fmt.Errorf("ps lstart: %w", err)
	}
	layout := "Mon Jan _2 15:04:05 2006"
	t, err := time.Parse(layout, strings.TrimSpace(string(out)))
	if err != nil {
		return 0, err
	}
	// Use Unix seconds as the portable identifier.
	return uint64(t.Unix()), nil
}

// processExe returns the executable path or name for a process on non-Linux
// Unix systems. It tries lsof first (full path on macOS/BSD), then falls back
// to ps comm= (basename).
func processExe(pid int) (string, error) {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid), "-Fn").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "n") {
				return strings.TrimPrefix(line, "n"), nil
			}
		}
	}
	out, err = exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return "", fmt.Errorf("ps comm: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
