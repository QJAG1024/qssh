package cmd

import (
	"fmt"
	"os"

	"qssh/internal/i18n"
)

// Delete removes a profile.
func Delete(name string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
		os.Exit(1)
	}

	if _, exists := s.Get(name); !exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
		os.Exit(1)
	}

	if err := s.Delete(name); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("profile.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf(i18n.T("profile.deleted")+"\n", name)
}