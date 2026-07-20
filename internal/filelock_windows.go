//go:build windows

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// FileLock holds an exclusive LockFileEx on a companion lock file.
type FileLock struct {
	f *os.File
}

// Lock acquires an exclusive lock on path+".lock", creating the file
// if needed. Blocks (with timeout) until the lock is free.
func Lock(path string) (*FileLock, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("filelock mkdir: %w", err)
	}
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("filelock open: %w", err)
	}

	// Lock the whole file. Retry until timeout so callers don't hang forever.
	var ol windows.Overlapped
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		err := windows.LockFileEx(
			windows.Handle(f.Fd()),
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0,
			1, 0, // lock 1 byte
			&ol,
		)
		if err == nil {
			return &FileLock{f: f}, nil
		}
		lastErr = err
		// ERROR_LOCK_VIOLATION / ERROR_IO_PENDING mean someone else holds it.
		time.Sleep(50 * time.Millisecond)
	}
	f.Close()
	return nil, fmt.Errorf("filelock timeout: %w", lastErr)
}

// Unlock releases the LockFileEx and closes the lock file.
func (l *FileLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	var ol windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &ol)
	err := l.f.Close()
	l.f = nil
	return err
}
