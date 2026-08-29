//go:build windows

package cmd

// umask is a no-op on Windows: Unix socket files do not exist there, and
// NT ACLs govern file access. The daemon socket path is never used on
// Windows (see daemon_stub_windows.go).
func umask(mask int) int { return mask }
