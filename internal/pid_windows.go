//go:build windows

package internal

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// ProcessIdentity identifies a process beyond a bare PID.
type ProcessIdentity struct {
	PID       int    `json:"pid"`
	StartTime uint64 `json:"start_time,omitempty"`
	Exe       string `json:"exe,omitempty"`
}

// CurrentIdentity returns identity fields for the running process.
func CurrentIdentity() ProcessIdentity {
	id := ProcessIdentity{PID: os.Getpid()}
	if st, err := processStartTime(id.PID); err == nil {
		id.StartTime = st
	}
	if exe, err := os.Executable(); err == nil {
		id.Exe = exe
	}
	return id
}

func processStartTime(pid int) (uint64, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)

	var creationTime, exitTime, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(h, &creationTime, &exitTime, &kernelTime, &userTime); err != nil {
		return 0, err
	}
	// FILETIME is 100-nanosecond intervals since 1601-01-01.
	// Convert to Unix seconds.
	ft := uint64(creationTime.HighDateTime)<<32 | uint64(creationTime.LowDateTime)
	sinceEpoch := ft/10_000_000 - 11644473600
	return sinceEpoch, nil
}

func processExe(pid int) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	var pathLen uint32 = windows.MAX_PATH
	buf := make([]uint16, pathLen)
	for {
		err := windows.QueryFullProcessImageName(h, 0, &buf[0], &pathLen)
		if err == nil {
			return syscall.UTF16ToString(buf[:pathLen]), nil
		}
		// Buffer too small? Grow and retry.
		if err != windows.ERROR_INSUFFICIENT_BUFFER {
			return "", err
		}
		pathLen *= 2
		buf = make([]uint16, pathLen)
	}
}

// Deprecated: SafePID validates by UID only. Use MatchIdentity instead.
func SafePID(pid int) error {
	return MatchIdentity(ProcessIdentity{PID: pid})
}

func MatchIdentity(id ProcessIdentity) error {
	if id.PID <= 1 {
		return fmt.Errorf("invalid pid %d (must be > 1)", id.PID)
	}
	isSelf := os.Getpid() == id.PID
	if isSelf && id.StartTime == 0 && id.Exe == "" {
		return nil
	}
	// Open with QUERY_LIMITED_INFORMATION to verify the process exists and to
	// read start time / executable path.
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(id.PID))
	if err != nil {
		return fmt.Errorf("process %d not reachable: %w", id.PID, err)
	}
	defer windows.CloseHandle(h)

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

// Deprecated: GracefulStop uses bare PID. Use GracefulStopIdent instead.
func GracefulStop(pid int) error {
	return GracefulStopIdent(ProcessIdentity{PID: pid})
}

func GracefulStopIdent(id ProcessIdentity) error {
	if err := MatchIdentity(id); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(id.PID))
	if err != nil {
		return fmt.Errorf("open process %d: %w", id.PID, err)
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("terminate process %d: %w", id.PID, err)
	}
	// Brief wait so the OS reaps it before callers re-check state.
	time.Sleep(50 * time.Millisecond)
	return nil
}

// sameExe compares executable paths, allowing basenames as a weak fallback.
func sameExe(a, b string) bool {
	if a == b {
		return true
	}
	ba := a
	bb := b
	if i := lastIndex(a, "\\"); i >= 0 {
		ba = a[i+1:]
	}
	if i := lastIndex(a, "/"); i >= 0 {
		ba = a[i+1:]
	}
	if i := lastIndex(b, "\\"); i >= 0 {
		bb = b[i+1:]
	}
	if i := lastIndex(b, "/"); i >= 0 {
		bb = b[i+1:]
	}
	ba = strings.TrimSuffix(ba, " (deleted)")
	bb = strings.TrimSuffix(bb, " (deleted)")
	return ba != "" && ba == bb
}

func lastIndex(s, c string) int {
	idx := -1
	for i := 0; i <= len(s)-len(c); i++ {
		if s[i:i+len(c)] == c {
			idx = i
		}
	}
	return idx
}
