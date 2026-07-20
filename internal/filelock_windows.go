//go:build windows

package internal

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// FileLock holds a Windows named mutex. The mutex name is derived from the
// absolute path of the protected file, so all processes/threads that lock the
// same file serialize through the same kernel object.
type FileLock struct {
	mu windows.Handle
}

// Lock acquires an exclusive lock. Blocks until the lock is free or abandoned.
func Lock(path string) (*FileLock, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("filelock abs: %w", err)
	}
	// Named mutex names may not contain path separators; use a hash of the path.
	sum := sha256.Sum256([]byte(abs))
	name := fmt.Sprintf("Local\\qssh-lock-%x", sum[:16])

	h, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(name))
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return nil, fmt.Errorf("filelock create mutex: %w", err)
	}

	wait, err := windows.WaitForSingleObject(h, windows.INFINITE)
	if err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("filelock wait: %w", err)
	}
	if wait != windows.WAIT_OBJECT_0 && wait != windows.WAIT_ABANDONED {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("filelock unexpected wait result: %d", wait)
	}
	return &FileLock{mu: h}, nil
}

// Unlock releases the mutex and closes its handle.
func (l *FileLock) Unlock() error {
	if l == nil || l.mu == 0 {
		return nil
	}
	windows.ReleaseMutex(l.mu)
	windows.CloseHandle(l.mu)
	l.mu = 0
	return nil
}
