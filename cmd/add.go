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

// AddOpts holds optional pre-filled values for non-interactive profile creation.
// Zero values mean "prompt the user interactively".
type AddOpts struct {
	Name, Host, User, Auth, Password, KeyPath string
	Port                                     int
}

// Add creates a new SSH credential profile.
// When any --host/--user/--auth flag is provided, all required fields must be
// specified via flags — no interactive fallback (avoids hanging AI agents).
func Add(opts AddOpts) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error", err), err)
		os.Exit(1)
	}

	name := opts.Name

	// Check if profile already exists.
	if _, exists := s.Get(name); exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.exists")+"\n", name)
		os.Exit(1)
	}

	// Determine mode: non-interactive when any structing flag is provided.
	nonInteractive := opts.Host != "" || opts.User != "" || opts.Auth != ""

	p := store.Profile{Name: name}

	if nonInteractive {
		if opts.Host == "" {
			fmt.Fprintln(os.Stderr, i18n.T("field.required_host"))
			os.Exit(1)
		}
		p.Host = opts.Host
	} else {
		p.Host = internal.Prompt("Host", "")
		if p.Host == "" {
			fmt.Fprintln(os.Stderr, i18n.T("field.required_host"))
			os.Exit(1)
		}
	}

	if opts.Port > 0 {
		p.Port = opts.Port
	} else if nonInteractive {
		p.Port = 22
	} else {
		portStr := internal.Prompt("Port", "22")
		p.Port, _ = strconv.Atoi(portStr)
	}

	if nonInteractive {
		if opts.User == "" {
			fmt.Fprintln(os.Stderr, i18n.T("field.required_user"))
			os.Exit(1)
		}
		p.User = opts.User
	} else {
		p.User = internal.Prompt("User", "")
		if p.User == "" {
			fmt.Fprintln(os.Stderr, i18n.T("field.required_user"))
			os.Exit(1)
		}
	}

	authStr := opts.Auth
	if authStr == "" && nonInteractive {
		authStr = "password"
	} else if authStr == "" {
		authStr = internal.Prompt("Auth method (password/key/agent)", "password")
	}
	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		if opts.Password != "" {
			p.Password = opts.Password
		} else if nonInteractive {
			fmt.Fprintln(os.Stderr, i18n.T("add.required_password"))
			os.Exit(1)
		} else {
			pass, err := internal.ReadPassword("Password")
			if err != nil {
				fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
				os.Exit(1)
			}
			p.Password = pass
		}
	case "key", "k":
		p.Auth = store.AuthKey
		if opts.KeyPath != "" {
			p.KeyPath = opts.KeyPath
		} else if nonInteractive {
			fmt.Fprintln(os.Stderr, i18n.T("add.required_keypath"))
			os.Exit(1)
		} else {
			p.KeyPath = internal.Prompt("Key path", "~/.ssh/id_ed25519")
		}
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