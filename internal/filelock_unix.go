//go:build !windows

package internal

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// FileLock holds an exclusive flock on a companion lock file.
// Use around full read-modify-write transactions that must be
// cross-process safe (store.json, config.json).
type FileLock struct {
	f *os.File
}

// Lock acquires an exclusive flock on path+".lock", creating the
// lock file if needed (mode 0600). Blocks until the lock is free.
// Call Unlock (or the returned unlock func) when done.
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
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("filelock flock: %w", err)
	}
	return &FileLock{f: f}, nil
}

// Unlock releases the flock and closes the lock file.
func (l *FileLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
	cerr := l.f.Close()
	l.f = nil
	if err != nil {
		return err
	}
	return cerr
}
