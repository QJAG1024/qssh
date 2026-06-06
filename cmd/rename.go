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

	p, exists := s.Get(oldName)
	if !exists {
		fmt.Fprintf(os.Stderr, "Profile %q not found.\n", oldName)
		os.Exit(1)
	}

	if _, exists := s.Get(newName); exists {
		fmt.Fprintf(os.Stderr, "Profile %q already exists.\n", newName)
		os.Exit(1)
	}

	if err := s.Delete(oldName); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting original: %v\n", err)
		os.Exit(1)
	}

	p.Name = newName
	if err := s.Add(p); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving renamed profile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Profile %q renamed to %q.\n", oldName, newName)
}