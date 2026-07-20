//go:build !windows

package internal

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

// ProcessIdentity identifies a process beyond a bare PID so we do not
// signal a recycled PID that now belongs to an unrelated process.
type ProcessIdentity struct {
	PID       int    `json:"pid"`
	StartTime uint64 `json:"start_time,omitempty"` // Linux /proc starttime (clock ticks); 0 = unknown
	Exe       string `json:"exe,omitempty"`        // resolved executable path, optional
}

// CurrentIdentity returns identity fields for the running process.
func CurrentIdentity() ProcessIdentity {
	id := ProcessIdentity{PID: os.Getpid()}
	if st, err := processStartTime(id.PID); err == nil {
		id.StartTime = st
	}
	if exe, err := os.Executable(); err == nil {
		id.Exe = exe
	} else if exe, err := processExe(id.PID); err == nil {
		id.Exe = exe
	}
	return id
}

// Deprecated: SafePID validates a PID by UID only, without start-time or exe
// verification. Use MatchIdentity with a full ProcessIdentity instead so
// recycled PIDs are not mistaken for the original process.
func SafePID(pid int) error {
	return MatchIdentity(ProcessIdentity{PID: pid})
}

// MatchIdentity checks that pid is live, owned by the current user, and
// (when provided) matches the recorded start time and executable.
// Self-checks still verify start-time/exe when provided so a stale identity
// record cannot accidentally match the current process.
func MatchIdentity(id ProcessIdentity) error {
	if id.PID <= 1 {
		return fmt.Errorf("invalid pid %d (must be > 1)", id.PID)
	}
	isSelf := os.Getpid() == id.PID
	if isSelf && id.StartTime == 0 && id.Exe == "" {
		return nil
	}

	// Linux: verify via /proc.
	statusPath := fmt.Sprintf("/proc/%d/status", id.PID)
	if _, err := os.Stat(statusPath); err == nil {
		ourUID := os.Getuid()
		if !isSelf {
			data, err := os.ReadFile(statusPath)
			if err != nil {
				return fmt.Errorf("cannot read proc status for pid %d: %w", id.PID, err)
			}
			uidOK := false
			for _, line := range splitLinesByte(data) {
				if len(line) > 5 && string(line[:4]) == "Uid:" {
					var uid int
					if _, err := fmt.Sscanf(string(line), "Uid:\t%d", &uid); err == nil {
						if uid != ourUID {
							return fmt.Errorf("pid %d belongs to uid %d, not current user %d", id.PID, uid, ourUID)
						}
						uidOK = true
						break
					}
				}
			}
			if !uidOK {
				return fmt.Errorf("cannot determine uid for pid %d", id.PID)
			}
		}
		if id.StartTime != 0 {
			st, err := processStartTime(id.PID)
			if err != nil {
				return fmt.Errorf("cannot read starttime for pid %d: %w", id.PID, err)
			}
			if st != id.StartTime {
				return fmt.Errorf("pid %d starttime mismatch (want %d got %d; PID reused?)", id.PID, id.StartTime, st)
			}
		}
		if id.Exe != "" {
			exe, err := processExe(id.PID)
			if err == nil && exe != "" && exe != id.Exe {
				// Also allow if one path is a resolved version of the other.
				if !sameExe(exe, id.Exe) {
					return fmt.Errorf("pid %d exe mismatch (want %q got %q; PID reused?)", id.PID, id.Exe, exe)
				}
			}
		}
		return nil
	}

	// Non-Linux: best-effort existence check, then start-time/exe verification.
	proc, err := os.FindProcess(id.PID)
	if err != nil {
		return fmt.Errorf("cannot find process %d: %w", id.PID, err)
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process %d not reachable: %w", id.PID, err)
	}
	if id.StartTime != 0 {
		st, err := processStartTime(id.PID)
		if err != nil {
			return fmt.Errorf("cannot read starttime for pid %d: %w", id.PID, err)
		}
		if st != id.StartTime {
			return fmt.Errorf("pid %d starttime mismatch (want %d got %d; PID reused?)", id.PID, id.StartTime, st)
		}
	}
	if id.Exe != "" {
		exe, err := processExe(id.PID)
		if err == nil && exe != "" && !sameExe(exe, id.Exe) {
			return fmt.Errorf("pid %d exe mismatch (want %q got %q; PID reused?)", id.PID, id.Exe, exe)
		}
	}
	return nil
}

// Deprecated: GracefulStop signals a process by bare PID without verifying
// start-time or executable identity. Use GracefulStopIdent with a full
// ProcessIdentity to avoid killing a recycled PID.
func GracefulStop(pid int) error {
	return GracefulStopIdent(ProcessIdentity{PID: pid})
}

// GracefulStopIdent validates identity then stops the process.
func GracefulStopIdent(id ProcessIdentity) error {
	if err := MatchIdentity(id); err != nil {
		return err
	}
	proc, err := os.FindProcess(id.PID)
	if err != nil {
		return err
	}
	_ = proc.Signal(syscall.SIGTERM)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		// Re-check identity: if the process exited, MatchIdentity fails.
		if err := MatchIdentity(id); err != nil {
			return nil // gone (or no longer matches — either way, stop)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Still matching — force kill.
	if err := MatchIdentity(id); err != nil {
		return nil
	}
	_ = proc.Kill()
	// Brief wait after SIGKILL.
	time.Sleep(50 * time.Millisecond)
	return nil
}

func sameExe(a, b string) bool {
	if a == b {
		return true
	}
	// Compare basenames as weak fallback (symlinks / deleted suffix).
	ba := a
	bb := b
	if i := strings.LastIndex(a, "/"); i >= 0 {
		ba = a[i+1:]
	}
	if i := strings.LastIndex(b, "/"); i >= 0 {
		bb = b[i+1:]
	}
	// Strip " (deleted)" suffix Linux adds.
	ba = strings.TrimSuffix(ba, " (deleted)")
	bb = strings.TrimSuffix(bb, " (deleted)")
	return ba == bb && ba != ""
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
