package cmd

import (
	"fmt"
	"os"

	"qssh/internal"
	"qssh/mount"
)

func init() {
	// Wire up store opener for mount daemon.
	mount.SetOpenStore(openStore)
}

// Mount launches a WebDAV mount daemon in the background.
func Mount(name, mountPoint string) {
	if err := mount.Mount(name, mountPoint); err != nil {
		fmt.Fprintf(os.Stderr, "  Mount failed: %v\n", err)
		os.Exit(1)
	}
}

// MountDaemon is the hidden entry point for the mount worker.
func MountDaemon(name, port string) {
	mount.MountDaemon(name, port)
}

// Unmount stops a mount daemon by profile name.
func Unmount(name string) {
	if err := mount.Unmount(name); err != nil {
		fmt.Fprintf(os.Stderr, "  Unmount failed: %v\n", err)
		os.Exit(1)
	}
	internal.RenderProgress(internal.StepResult{
		ID: internal.StepShellStart, Status: internal.StepDone,
		Message: "Unmounted",
	})
}