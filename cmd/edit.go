package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"qssh/internal"
	"qssh/store"
)

// Edit loads an existing profile and allows interactive modification.
func Edit(name string) {
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

	fmt.Printf("Editing profile %q (press Enter to keep current value)\n", name)
	fmt.Println()

	host := internal.Prompt("Host", p.Host)
	if host != "" {
		p.Host = host
	}

	portStr := internal.Prompt("Port", strconv.Itoa(p.Port))
	if portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			p.Port = port
		}
	}

	user := internal.Prompt("User", p.User)
	if user != "" {
		p.User = user
	}

	authStr := internal.Prompt("Auth method (password/key/agent)", string(p.Auth))
	if authStr != "" {
		switch strings.ToLower(authStr) {
		case "password", "p":
			p.Auth = store.AuthPassword
		case "key", "k":
			p.Auth = store.AuthKey
		case "agent", "a":
			p.Auth = store.AuthAgent
		case "keyboard-interactive", "ki":
			p.Auth = store.AuthKeyboardInteractive
		default:
			fmt.Fprintf(os.Stderr, "Unsupported auth method %q\n", authStr)
			os.Exit(1)
		}
	}

	switch p.Auth {
	case store.AuthPassword:
		changePass := internal.Confirm("Change password?", false)
		if changePass {
			pass, err := internal.ReadPassword("New password")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
				os.Exit(1)
			}
			p.Password = pass
		}
	case store.AuthKey:
		keyPath := internal.Prompt("Key path", p.KeyPath)
		if keyPath != "" {
			p.KeyPath = keyPath
		}
	}

	p.SetDefaults()
	if err := s.Update(name, p); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving profile: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Profile %q updated.\n", name)
}