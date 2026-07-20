//go:build windows

package internal

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
)

// FileLock holds a Windows named mutex. The mutex name is derived from the
// absolute, lower-cased path of the protected file, so all processes/threads
// that lock the same file serialize through the same kernel object.
type FileLock struct {
	mu windows.Handle
}

// Lock acquires an exclusive lock. Blocks until the lock is free or abandoned.
// Windows mutex ownership is tied to the acquiring OS thread, so the calling
// goroutine is pinned to that thread until Unlock.
func Lock(path string) (*FileLock, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("filelock abs: %w", err)
	}
	// Case-normalize so different-cased paths to the same file share one lock.
	abs = strings.ToLower(abs)
	sum := sha256.Sum256([]byte(abs))
	name := fmt.Sprintf("Local\\qssh-lock-%x", sum[:16])

	// Pin to the current OS thread; the thread that acquires the mutex must
	// be the thread that releases it.
	runtime.LockOSThread()

	h, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(name))
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("filelock create mutex: %w", err)
	}

	wait, err := windows.WaitForSingleObject(h, windows.INFINITE)
	if err != nil {
		windows.CloseHandle(h)
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("filelock wait: %w", err)
	}
	if wait != windows.WAIT_OBJECT_0 && wait != windows.WAIT_ABANDONED {
		windows.CloseHandle(h)
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("filelock unexpected wait result: %d", wait)
	}
	return &FileLock{mu: h}, nil
}

// Unlock releases the mutex, closes its handle, and unpins the goroutine from
// the OS thread that acquired it.
func (l *FileLock) Unlock() error {
	if l == nil || l.mu == 0 {
		runtime.UnlockOSThread()
		return nil
	}
	err := windows.ReleaseMutex(l.mu)
	windows.CloseHandle(l.mu)
	l.mu = 0
	runtime.UnlockOSThread()
	if err != nil {
		return fmt.Errorf("filelock release mutex: %w", err)
	}
	return nil
}
