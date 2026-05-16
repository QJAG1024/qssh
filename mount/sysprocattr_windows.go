//go:build windows

package mount

import "syscall"

// daemonSysProcAttr returns SysProcAttr for the mount daemon process.
// On Windows, Setpgid is not available.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}