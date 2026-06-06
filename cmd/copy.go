package cmd

import (
	"fmt"
	"os"

	"qssh/internal/i18n"
)

// Copy creates a new profile from an existing one.
func Copy(oldName, newName string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error"), err)
		os.Exit(1)
	}

	p, exists := s.Get(oldName)
	if !exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", oldName)
		os.Exit(1)
	}

	if _, exists := s.Get(newName); exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.exists")+"\n", newName)
		os.Exit(1)
	}

	newP := p.Copy()
	newP.Name = newName
	newP.CreatedAt = p.CreatedAt
	newP.UpdatedAt = p.UpdatedAt
	newP.LastUsed = p.LastUsed
	newP.ConnectionCount = 0

	if err := s.Add(newP); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying profile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Profile %q copied to %q.\n", oldName, newName)
}