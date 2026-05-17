package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/store"
)

// Add creates a new SSH credential profile via interactive prompts.
func Add(name string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error", err), err)
		os.Exit(1)
	}

	// Check if profile already exists.
	if _, exists := s.Get(name); exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.exists")+"\n", name)
		os.Exit(1)
	}

	p := store.Profile{Name: name}

	p.Host = internal.Prompt("Host", "")
	if p.Host == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_host"))
		os.Exit(1)
	}

	portStr := internal.Prompt("Port", "22")
	p.Port, _ = strconv.Atoi(portStr)

	p.User = internal.Prompt("User", "")
	if p.User == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_user"))
		os.Exit(1)
	}

	authStr := internal.Prompt("Auth method (password/key/agent)", "password")
	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		pass, err := internal.ReadPassword("Password")
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
			os.Exit(1)
		}
		p.Password = pass
	case "key", "k":
		p.Auth = store.AuthKey
		p.KeyPath = internal.Prompt("Key path", "~/.ssh/id_ed25519")
	case "agent", "a":
		p.Auth = store.AuthAgent
	case "keyboard-interactive", "ki":
		p.Auth = store.AuthKeyboardInteractive
	default:
		fmt.Fprintf(os.Stderr, i18n.T("auth.unsupported")+"\n", authStr)
		os.Exit(1)
	}

	p.SetDefaults()
	if err := s.Add(p); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("profile.save_error")+"\n", err)
		os.Exit(1)
	}

	fmt.Printf(i18n.T("profile.created")+"\n", name, name)
}