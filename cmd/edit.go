package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sftpproxy"
	"qssh/store"
)

// Edit loads an existing profile and allows modification.
// When any structing flag (Host/User/Auth/Port/etc.) is set, it runs in
// non-interactive mode — only the specified fields are updated.
func Edit(name string, opts AddOpts) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
		os.Exit(1)
	}

	p, exists := s.Get(name)
	if !exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
		os.Exit(1)
	}
	orig := p.Copy()

	nonInteractive := opts.Host != "" || opts.User != "" || opts.Auth != "" ||
		opts.Port > 0 || opts.Password != "" || opts.KeyPath != "" ||
		opts.KeyPassphrase != "" || opts.Proxy != "" || opts.Options != nil ||
		len(opts.Tags) > 0

	if nonInteractive {
		editNonInteractive(&p, opts)
	} else {
		editInteractive(s, &p)
	}

	p.SetDefaults()
	if err := s.Update(name, p); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("profile.save_error")+"\n", err)
		os.Exit(1)
	}
	// Revoke live daemon if connection-defining fields changed.
	if connectionIdentityChanged(orig, p) {
		if err := stopDaemon(name); err != nil && daemonRunning(name) {
			fmt.Fprintf(os.Stderr, "warning: could not stop daemon for %q (it will auto-terminate within 30s): %v\n", name, err)
		}
		_ = os.Remove(daemonSocketPath(name))
		_ = os.Remove(daemonPidPath(name))
		_ = sftpproxy.Stop(name)
	}
	fmt.Printf(i18n.T("profile.updated")+"\n", name)
}

func editNonInteractive(p *store.Profile, opts AddOpts) {
	if opts.Host != "" {
		p.Host = opts.Host
	}
	if opts.Port > 0 {
		p.Port = opts.Port
	}
	if opts.User != "" {
		p.User = opts.User
	}
	if opts.Auth != "" {
		switch strings.ToLower(opts.Auth) {
		case "password", "p":
			p.Auth = store.AuthPassword
		case "key", "k":
			p.Auth = store.AuthKey
		case "agent", "a":
			p.Auth = store.AuthAgent
		case "keyboard-interactive", "ki":
			p.Auth = store.AuthKeyboardInteractive
		default:
			fmt.Fprintf(os.Stderr, i18n.T("auth.unsupported")+"\n", opts.Auth)
			os.Exit(1)
		}
	}
	switch p.Auth {
	case store.AuthPassword:
		if opts.Password != "" {
			p.Password = opts.Password
		}
	case store.AuthKey:
		if opts.KeyPath != "" {
			p.KeyPath = opts.KeyPath
		}
		if opts.KeyPassphrase != "" {
			p.KeyPassphrase = opts.KeyPassphrase
		}
	}
	if opts.Options != nil {
		var err error
		p.Options, err = internal.ApplyOptionMap(p.Options, opts.Options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "options: %v\n", err)
			os.Exit(1)
		}
	}
	if opts.Proxy != "" {
		p.Proxy = opts.Proxy
	}
	if len(opts.Tags) > 0 {
		p.Tags = opts.Tags
	}
	if opts.Name != "" {
		p.Name = opts.Name
	}
}

func editInteractive(s *store.Store, p *store.Profile) {
	menuItems := []string{
		"Host/Port/User",
		"Auth method & credentials",
		"ProxyJump",
		"Options",
		"Tags",
		"Save & exit",
		"Discard",
	}

	for {
		fmt.Println()
		fmt.Println(strings.Repeat("─", 50))
		fmt.Printf("  Editing: %s  (%s@%s:%d)\n", p.Name, p.User, p.Host, p.Port)
		fmt.Println(strings.Repeat("─", 50))
		for i, item := range menuItems {
			fmt.Printf("  %d) %s\n", i+1, item)
		}
		fmt.Println()

		choice := internal.Prompt("Choose", "")
		if choice == "" {
			choice = "6" // default: Save & exit
		}

		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(menuItems) {
			fmt.Println("Invalid choice")
			continue
		}

		switch n {
		case 1: // Host/Port/User
			host := internal.Prompt("Host", p.Host)
			if host != "" {
				p.Host = host
			}
			portStr := internal.Prompt("Port", strconv.Itoa(p.Port))
			if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
				p.Port = port
			}
			user := internal.Prompt("User", p.User)
			if user != "" {
				p.User = user
			}
		case 2: // Auth
			editAuth(p)
		case 3: // Proxy
			proxy := internal.Prompt("ProxyJump profile (empty to remove)", p.Proxy)
			if proxy == "" && p.Proxy != "" {
				if internal.Confirm("Remove proxy?", false) {
					p.Proxy = ""
				}
			} else if proxy != "" {
				p.Proxy = proxy
			}
		case 4: // Options
			optStr := internal.Prompt("Options (comma-separated KEY=VALUE)", optionString(p.Options))
			if optStr == "" && len(p.Options) > 0 {
				if internal.Confirm("Remove all options?", false) {
					p.Options = nil
				}
			} else if optStr != "" {
				parsed := parseOptionString(optStr)
				var err error
				p.Options, err = internal.ApplyOptionMap(p.Options, parsed)
				if err != nil {
					fmt.Fprintf(os.Stderr, "options: %v\n", err)
					os.Exit(1)
				}
			}
		case 5: // Tags
			tagsStr := internal.Prompt("Tags (comma-separated)", strings.Join(p.Tags, ", "))
			if tagsStr != "" {
				p.Tags = parseTags(tagsStr)
			}
		case 6: // Save & exit
			printSummary(*p)
			if internal.Confirm("Save changes?", true) {
				return
			}
		case 7: // Discard
			if internal.Confirm("Discard all changes?", false) {
				fmt.Println(i18n.T("profile.cancelled"))
				os.Exit(0)
			}
		}
	}
}

func editAuth(p *store.Profile) {
	fmt.Println()
	currentAuth := string(p.Auth)
	authMethods := validAuthMethods
	authStr := internal.SelectPrompt("Auth method:", authMethods, currentAuth)

	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		if internal.Confirm("Change password?", false) {
			pass, err := internal.ReadPasswordWithConfirm("New password")
			if err != nil {
				fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
				os.Exit(1)
			}
			p.Password = pass
		}
	case "key", "k":
		p.Auth = store.AuthKey
		keyPath := internal.Prompt("Key path", p.KeyPath)
		if keyPath != "" {
			p.KeyPath = internal.ExpandPath(keyPath)
		}
		if internal.Confirm("Key has passphrase?", p.KeyPassphrase != "") {
			pass, err := internal.ReadPassword("Key passphrase")
			if err != nil {
				fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
				os.Exit(1)
			}
			p.KeyPassphrase = pass
		} else {
			p.KeyPassphrase = ""
		}
	case "agent", "a":
		p.Auth = store.AuthAgent
		p.Password = ""
		p.KeyPath = ""
		p.KeyPassphrase = ""
	case "keyboard-interactive", "ki":
		p.Auth = store.AuthKeyboardInteractive
	}
}

// connectionIdentityChanged reports whether host/port/user/auth/key/proxy
// (or the credentials themselves) differ enough that a live daemon session
// must be discarded.
func connectionIdentityChanged(a, b store.Profile) bool {
	if a.Host != b.Host || a.Port != b.Port || a.User != b.User {
		return true
	}
	if a.Auth != b.Auth || a.Proxy != b.Proxy {
		return true
	}
	if a.Password != b.Password || a.KeyPath != b.KeyPath || a.KeyPassphrase != b.KeyPassphrase {
		return true
	}
	return false
}

// optionString converts a map to a comma-separated KEY=VALUE string.
func optionString(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	var pairs []string
	for k, v := range m {
		pairs = append(pairs, k+"="+v)
	}
	return strings.Join(pairs, ",")
}

// parseOptionString parses a comma-separated KEY=VALUE string.
func parseOptionString(s string) map[string]string {
	if s == "" {
		return nil
	}
	m := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if key != "" {
			m[key] = val
		}
	}
	return m
}
