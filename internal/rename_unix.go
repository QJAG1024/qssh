//go:build !windows

package internal

import "os"

// ReplaceFile is a thin wrapper around os.Rename on Unix-like systems.
func ReplaceFile(src, dst string) error {
	return os.Rename(src, dst)
}
