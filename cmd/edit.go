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

	nonInteractive := opts.Host != "" || opts.User != "" || opts.Auth != "" ||
		opts.Port > 0 || opts.Password != "" || opts.KeyPath != "" ||
		opts.Proxy != "" || opts.Options != nil

	if nonInteractive {
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
		}
		if opts.Options != nil {
			if p.Options == nil {
				p.Options = make(map[string]string, len(opts.Options))
			}
			for k, v := range opts.Options {
				p.Options[k] = v
			}
		}
		if opts.Proxy != "" {
			p.Proxy = opts.Proxy
		}
		if opts.Name != "" {
			p.Name = opts.Name
		}
	} else {
		fmt.Printf(i18n.T("field.edit_header")+"\n", name)
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
				fmt.Fprintf(os.Stderr, i18n.T("auth.unsupported")+"\n", authStr)
				os.Exit(1)
			}
		}

		switch p.Auth {
		case store.AuthPassword:
			changePass := internal.Confirm("Change password?", false)
			if changePass {
				pass, err := internal.ReadPassword("New password")
				if err != nil {
					fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
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

		proxy := internal.Prompt("Proxy profile (or leave empty for none)", p.Proxy)
		if proxy != "" {
			p.Proxy = proxy
		} else if proxy == "" && p.Proxy != "" && !internal.Confirm("Remove proxy?", false) {
			// keep existing
		} else {
			p.Proxy = ""
		}

		optStr := internal.Prompt("Options (comma-separated KEY=VALUE, or leave empty)", optionString(p.Options))
		if optStr != "" {
			parsed := parseOptionString(optStr)
			if p.Options == nil {
				p.Options = make(map[string]string, len(parsed))
			}
			for k, v := range parsed {
				p.Options[k] = v
			}
		} else if optStr == "" && len(p.Options) > 0 && !internal.Confirm("Remove all options?", false) {
			// keep existing
		} else {
			p.Options = nil
		}
	}

	p.SetDefaults()
	if err := s.Update(name, p); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("profile.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf(i18n.T("profile.updated")+"\n", name)
}

// optionString converts a map to a comma-separated KEY=VALUE string for prompting.
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