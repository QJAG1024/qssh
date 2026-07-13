package cmd

import (
	"fmt"
	"os"
)

// Rename renames a profile.
func Rename(oldName, newName string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}

	if err := s.Rename(oldName, newName); err != nil {
		fmt.Fprintf(os.Stderr, "Error renaming profile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Profile %q renamed to %q.\n", oldName, newName)
}
