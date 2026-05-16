package cmd

import (
	"fmt"
	"os"

	"qssh/internal"
	"qssh/mount"
)

// Mount connects to a profile and mounts it via WebDAV.
func Mount(name string, mountPoint string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}

	p, exists := s.Get(name)
	if !exists {
		fmt.Fprintf(os.Stderr, "Profile %q not found.\n", name)
		os.Exit(1)
	}

	if err := mount.Mount(p, mountPoint); err != nil {
		fmt.Fprintf(os.Stderr, "  Mount failed: %v\n", err)
		os.Exit(1)
	}
}

// Unmount unmounts a previously mounted WebDAV filesystem.
func Unmount(mountPoint string) {
	if err := mount.Unmount(mountPoint); err != nil {
		fmt.Fprintf(os.Stderr, "  Unmount failed: %v\n", err)
		os.Exit(1)
	}
	internal.RenderProgress(internal.StepResult{
		ID: internal.StepShellStart, Status: internal.StepDone,
		Message: "Unmounted",
	})
}