//go:build windows

package internal

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// ReplaceFile atomically replaces dst with src. On Windows it retries when
// the destination is temporarily locked by another handle, which can happen
// even under an advisory lock because of in-kernel handle lifetime timing.
func ReplaceFile(src, dst string) error {
	const retries = 100
	for i := 0; i < retries; i++ {
		err := os.Rename(src, dst)
		if err == nil {
			return nil
		}
		if isLockedError(err) {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		return err
	}
	return os.Rename(src, dst)
}

func isLockedError(err error) bool {
	pathErr := &os.PathError{}
	if errors.As(err, &pathErr) {
		switch pathErr.Err {
		case windows.ERROR_SHARING_VIOLATION,
			windows.ERROR_LOCK_VIOLATION,
			windows.ERROR_ACCESS_DENIED:
			return true
		}
	}
	return false
}
