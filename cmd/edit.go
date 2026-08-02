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
		i18n.T("edit.menu.host"),
		i18n.T("edit.menu.auth"),
		i18n.T("edit.menu.proxy"),
		i18n.T("edit.menu.options"),
		i18n.T("edit.menu.tags"),
		i18n.T("edit.menu.save"),
		i18n.T("edit.menu.discard"),
	}

	for {
		fmt.Println()
		fmt.Println(strings.Repeat("─", 50))
		fmt.Printf(i18n.T("edit.panel.title")+"\n", p.Name, p.User, p.Host, p.Port)
		fmt.Println(strings.Repeat("─", 50))
		for i, item := range menuItems {
			fmt.Printf("  %d) %s\n", i+1, item)
		}
		fmt.Println()

		choice := internal.Prompt(i18n.T("edit.prompt.choose"), "")
		if choice == "" {
			choice = "6" // default: Save & exit
		}

		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(menuItems) {
			fmt.Println(i18n.T("edit.error.invalid"))
			continue
		}

		switch n {
		case 1: // Host/Port/User
			host := internal.Prompt(i18n.T("edit.prompt.host"), p.Host)
			if host != "" {
				p.Host = host
			}
			portStr := internal.Prompt(i18n.T("edit.prompt.port"), strconv.Itoa(p.Port))
			if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
				p.Port = port
			}
			user := internal.Prompt(i18n.T("edit.prompt.user"), p.User)
			if user != "" {
				p.User = user
			}
		case 2: // Auth
			editAuth(p)
		case 3: // Proxy
			proxy := internal.Prompt(i18n.T("edit.prompt.proxy"), p.Proxy)
			if proxy == "" && p.Proxy != "" {
				if internal.Confirm(i18n.T("edit.confirm.remove_proxy"), false) {
					p.Proxy = ""
				}
			} else if proxy != "" {
				p.Proxy = proxy
			}
		case 4: // Options
			// Interactive per-key panel for profile options.
			editPerProfileOptions(p)
		case 5: // Tags
			tagsStr := internal.Prompt(i18n.T("edit.prompt.tags"), strings.Join(p.Tags, ", "))
			if tagsStr != "" {
				p.Tags = parseTags(tagsStr)
			}
		case 6: // Save & exit
			printSummary(*p)
			if internal.Confirm(i18n.T("edit.confirm.save"), true) {
				return
			}
		case 7: // Discard
			if internal.Confirm(i18n.T("edit.confirm.discard"), false) {
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
	authStr := internal.SelectPrompt(i18n.T("edit.prompt.auth"), authMethods, currentAuth)

	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		if internal.Confirm(i18n.T("edit.confirm.changepass"), false) {
			pass, err := internal.ReadPasswordWithConfirm(i18n.T("edit.prompt.newpass"))
			if err != nil {
				fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
				os.Exit(1)
			}
			p.Password = pass
		}
	case "key", "k":
		p.Auth = store.AuthKey
		keyPath := internal.Prompt(i18n.T("edit.prompt.keypath"), p.KeyPath)
		if keyPath != "" {
			p.KeyPath = internal.ExpandPath(keyPath)
		}
		if internal.Confirm(i18n.T("edit.confirm.keypass"), p.KeyPassphrase != "") {
			pass, err := internal.ReadPassword(i18n.T("edit.prompt.keypass"))
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

// editPerProfileOptions is an interactive panel for setting per-profile
// option keys (the --set-option whitelist). Replaces the raw comma-string
// Options editing with per-key selection. Writes p.Options directly.
func editPerProfileOptions(p *store.Profile) {
	meta := perProfileKeysMeta()
	keys := perProfileKeysSorted()

	for {
		fmt.Println()
		fmt.Println(strings.Repeat("─", 50))
		fmt.Println("  " + i18n.T("options.panel.title"))
		fmt.Println(strings.Repeat("─", 50))
		for i, k := range keys {
			val := p.Options[k]
			display := val
			if display == "" {
				display = i18n.T("config.panel.not_set")
			}
			desc := ""
			if m, ok := meta[k]; ok && m.desc != "" {
				desc = "  ← " + i18n.T(m.desc)
			}
			fmt.Printf("  %2d) %-25s = %s%s\n", i+1, k, display, desc)
		}
		fmt.Println(strings.Repeat("─", 50))
		fmt.Println("  " + i18n.T("options.panel.set"))
		fmt.Println("  " + i18n.T("options.panel.unset"))
		fmt.Println("  " + i18n.T("options.panel.back"))
		fmt.Println()

		choice := internal.Prompt(i18n.T("config.panel.action"), "q")
		switch strings.ToLower(choice) {
		case "q", "":
			return
		case "s":
			setPerProfileKey(p, meta, keys)
		case "u":
			unsetPerProfileKey(p, meta, keys)
		default:
			// Numeric: jump to a key and edit it.
			n, err := strconv.Atoi(choice)
			if err == nil && n >= 1 && n <= len(keys) {
				setPerProfileKey(p, meta, keys, keys[n-1])
			}
		}
	}
}

// setPerProfileKey prompts for a key's value (SelectPrompt when the key has
// fixed options) and applies it to p.Options. An empty value clears the
// override. When keyHint is non-empty it targets that key directly.
func setPerProfileKey(p *store.Profile, meta map[string]configKeyMeta, keys []string, keyHint ...string) {
	var key string
	if len(keyHint) > 0 && keyHint[0] != "" {
		key = keyHint[0]
	} else {
		key = internal.Prompt(i18n.T("options.panel.key"), "")
		if key == "" {
			return
		}
	}
	m, ok := meta[key]
	if !ok {
		fmt.Fprintf(os.Stderr, i18n.T("options.error.unknown")+"\n", key)
		return
	}

	current := p.Options[key]
	var val string
	if len(m.options) > 0 {
		val = internal.SelectPrompt(i18n.T("options.panel.value")+":", m.options, current)
	} else {
		val = internal.Prompt(i18n.T("options.panel.value"), current)
	}

	if val == "" {
		// Empty clears the override.
		delete(p.Options, key)
		return
	}
	applied, err := internal.ApplyOptionMap(p.Options, map[string]string{key: val})
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("edit.error.options")+"\n", err)
		return
	}
	p.Options = applied
}

// unsetPerProfileKey clears a key's override.
func unsetPerProfileKey(p *store.Profile, meta map[string]configKeyMeta, keys []string) {
	key := internal.Prompt(i18n.T("options.panel.unset_which"), "")
	if key == "" {
		return
	}
	if _, ok := meta[key]; !ok {
		fmt.Fprintf(os.Stderr, i18n.T("options.error.unknown")+"\n", key)
		return
	}
	delete(p.Options, key)
}
