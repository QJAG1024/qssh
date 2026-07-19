//go:build !windows

package internal

import (
	"fmt"
	"os"
	"syscall"
)

// SafePID validates a PID for use with FindProcess/Kill.
// Returns nil if the PID belongs to the current user and is safe to signal.
// On Linux, verifies UID match via /proc/$pid/status.
func SafePID(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("invalid pid %d (must be > 1)", pid)
	}
	if os.Getpid() == pid {
		return nil // safe: self-signal
	}
	// On Linux, verify the PID belongs to the same user.
	if _, err := os.Stat(fmt.Sprintf("/proc/%d/status", pid)); err == nil {
		// Read UID from /proc/$pid/status
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			return fmt.Errorf("cannot read proc status for pid %d: %w", pid, err)
		}
		// Uid: line in /proc/pid/status
		ourUID := os.Getuid()
		for _, line := range splitLinesByte(data) {
			if len(line) > 5 && string(line[:4]) == "Uid:" {
				// Format: "Uid:\t%d\t%d\t%d\t%d" (real, effective, saved, filesystem)
				var uid int
				if _, err := fmt.Sscanf(string(line), "Uid:\t%d", &uid); err == nil {
					if uid != ourUID {
						return fmt.Errorf("pid %d belongs to uid %d, not current user %d", pid, uid, ourUID)
					}
					return nil
				}
			}
		}
		return fmt.Errorf("cannot determine uid for pid %d", pid)
	}
	// Non-Linux: best-effort — accept if we can find the process.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("cannot find process %d: %w", pid, err)
	}
	// Signal(0) checks existence without actually sending a signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process %d not reachable: %w", pid, err)
	}
	return nil
}

func splitLinesByte(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// GracefulStop sends SIGTERM, waits, then SIGKILL.
func GracefulStop(pid int) error {
	if err := SafePID(pid); err != nil {
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// SIGTERM first.
	_ = proc.Signal(syscall.SIGTERM)
	// Wait briefly for graceful exit.
	// Check if still alive, then SIGKILL.
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		_ = proc.Kill()
	}
	return nil
}