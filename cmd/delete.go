package cmd

import (
	"fmt"
	"os"

	"qssh/internal"
)

// Delete removes a profile after confirmation.
func Delete(name string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}

	if _, exists := s.Get(name); !exists {
		fmt.Fprintf(os.Stderr, "Profile %q not found.\n", name)
		os.Exit(1)
	}

	if !internal.Confirm(fmt.Sprintf("Delete profile %q?", name), false) {
		fmt.Println("Cancelled.")
		return
	}

	if err := s.Delete(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting profile: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Profile %q deleted.\n", name)
}