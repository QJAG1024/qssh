//go:build windows

package internal

import "fmt"

func SafePID(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("invalid pid %d (must be > 1)", pid)
	}
	return nil
}

func GracefulStop(pid int) error {
	if err := SafePID(pid); err != nil {
		return err
	}
	return fmt.Errorf("GracefulStop not implemented on Windows")
}