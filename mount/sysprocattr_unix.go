//go:build !windows

package mount

import "syscall"

// daemonSysProcAttr returns SysProcAttr for the mount daemon process.
// On Unix we set Setpgid to detach the child from the parent's process group.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}