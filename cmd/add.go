package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"qssh/internal"
	"qssh/store"
)

// Add creates a new SSH credential profile via interactive prompts.
func Add(name string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}

	// Check if profile already exists.
	if _, exists := s.Get(name); exists {
		fmt.Fprintf(os.Stderr, "Profile %q already exists. Use --edit to modify it.\n", name)
		os.Exit(1)
	}

	p := store.Profile{Name: name}

	p.Host = internal.Prompt("Host", "")
	if p.Host == "" {
		fmt.Fprintln(os.Stderr, "Host is required.")
		os.Exit(1)
	}

	portStr := internal.Prompt("Port", "22")
	p.Port, _ = strconv.Atoi(portStr)

	p.User = internal.Prompt("User", "")
	if p.User == "" {
		fmt.Fprintln(os.Stderr, "User is required.")
		os.Exit(1)
	}

	authStr := internal.Prompt("Auth method (password/key/agent)", "password")
	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		pass, err := internal.ReadPassword("Password")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Unsupported auth method %q\n", authStr)
		os.Exit(1)
	}

	p.SetDefaults()
	if err := s.Add(p); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving profile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Profile %q created. Use 'qssh %s' to connect.\n", name, name)
}