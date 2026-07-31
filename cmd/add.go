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
	Name, Host, User, Auth, Password, KeyPath, KeyPassphrase string
	Port                                                     int
	Proxy                                                    string
	Options                                                  map[string]string
	Tags                                                     []string
}

// Add creates a new SSH credential profile.
// When any --host/--user/--auth flag is provided, all required fields must be
// specified via flags — no interactive fallback (avoids hanging AI agents).
func Add(opts AddOpts) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
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
		addNonInteractive(&p, opts)
	} else {
		addInteractive(s, &p, opts)
	}

	p.SetDefaults()
	if err := s.Add(p); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("profile.save_error")+"\n", err)
		os.Exit(1)
	}

	fmt.Printf(i18n.T("profile.created")+"\n", name, name)
}

func addNonInteractive(p *store.Profile, opts AddOpts) {
	if opts.Host == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_host"))
		os.Exit(1)
	}
	p.Host = opts.Host

	if opts.Port > 0 {
		p.Port = opts.Port
	} else {
		p.Port = 22
	}

	if opts.User == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_user"))
		os.Exit(1)
	}
	p.User = opts.User

	authStr := opts.Auth
	if authStr == "" {
		authStr = "password"
	}
	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		if opts.Password != "" {
			p.Password = opts.Password
		} else {
			fmt.Fprintln(os.Stderr, i18n.T("add.required_password"))
			os.Exit(1)
		}
	case "key", "k":
		p.Auth = store.AuthKey
		if opts.KeyPath != "" {
			p.KeyPath = opts.KeyPath
		} else {
			fmt.Fprintln(os.Stderr, i18n.T("add.required_keypath"))
			os.Exit(1)
		}
		if opts.KeyPassphrase != "" {
			p.KeyPassphrase = opts.KeyPassphrase
		}
	case "agent", "a":
		p.Auth = store.AuthAgent
	case "keyboard-interactive", "ki":
		p.Auth = store.AuthKeyboardInteractive
	default:
		fmt.Fprintf(os.Stderr, i18n.T("auth.unsupported")+"\n", authStr)
		os.Exit(1)
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
}

func addInteractive(s *store.Store, p *store.Profile, opts AddOpts) {
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf(i18n.T("add.panel.title")+"\n", p.Name)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	// Host
	p.Host = internal.Prompt(i18n.T("add.prompt.host"), "")
	if p.Host == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_host"))
		os.Exit(1)
	}

	// Port
	portStr := internal.Prompt(i18n.T("add.prompt.port"), "22")
	p.Port, _ = strconv.Atoi(portStr)

	// User
	// User — default to current OS user
	defaultUser := os.Getenv("USER")
	if defaultUser == "" {
		defaultUser = os.Getenv("USERNAME")
	}
	p.User = internal.Prompt(i18n.T("add.prompt.user"), defaultUser)
	if p.User == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_user"))
		os.Exit(1)
	}

	// Auth method — numbered selection
	authMethods := validAuthMethods
	authStr := internal.SelectPrompt(i18n.T("add.prompt.auth"), authMethods, "password")
	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		pass, err := internal.ReadPasswordWithConfirm(i18n.T("add.prompt.password"))
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
			os.Exit(1)
		}
		p.Password = pass
	case "key", "k":
		p.Auth = store.AuthKey
		p.KeyPath = internal.Prompt(i18n.T("add.prompt.keypath"), "~/.ssh/id_ed25519")
		p.KeyPath = internal.ExpandPath(p.KeyPath)
		// Verify key file exists
		if _, err := os.Stat(p.KeyPath); err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("add.warn.key_missing")+"\n", p.KeyPath, err)
		}
		if internal.Confirm(i18n.T("add.confirm.keypass"), false) {
			pass, err := internal.ReadPassword(i18n.T("add.prompt.keypass"))
			if err != nil {
				fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
				os.Exit(1)
			}
			p.KeyPassphrase = pass
		}
	case "agent", "a":
		p.Auth = store.AuthAgent
	case "keyboard-interactive", "ki":
		p.Auth = store.AuthKeyboardInteractive
	}

	// Proxy (jump host)
	proxy := internal.Prompt(i18n.T("add.prompt.proxy"), "")
	if proxy != "" {
		// Validate proxy profile exists
		if _, exists := s.Get(proxy); !exists {
			fmt.Fprintf(os.Stderr, i18n.T("add.warn.proxy_missing")+"\n", proxy)
		}
		// Cycle detection: proxy cannot point to self
		if proxy == p.Name {
			fmt.Fprintln(os.Stderr, i18n.T("add.error.self_proxy"))
			os.Exit(1)
		}
		p.Proxy = proxy
	}

	// Options
	optStr := internal.Prompt(i18n.T("add.prompt.options"), "")
	if optStr != "" {
		parsed := parseOptionString(optStr)
		var err error
		p.Options, err = internal.ApplyOptionMap(p.Options, parsed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "options: %v\n", err)
			os.Exit(1)
		}
	}

	// Tags
	tagsStr := internal.Prompt(i18n.T("add.prompt.tags"), "")
	if tagsStr != "" {
		p.Tags = parseTags(tagsStr)
	}

	// Preview summary
	printSummary(*p)

	if !internal.Confirm(i18n.T("add.prompt.save"), true) {
		fmt.Println(i18n.T("profile.cancelled"))
		os.Exit(0)
	}
}

// parseTags splits a comma-separated string into trimmed tags.
func parseTags(s string) []string {
	if s == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// printSummary prints a profile summary before save, omitting secrets.
func printSummary(p store.Profile) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("  " + i18n.T("add.preview.title"))
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  %-11s %s\n", i18n.T("add.preview.name")+":", p.Name)
	fmt.Printf("  %-11s %s:%d\n", i18n.T("add.preview.host")+":", p.Host, p.Port)
	fmt.Printf("  %-11s %s\n", i18n.T("add.preview.user")+":", p.User)
	fmt.Printf("  %-11s %s\n", i18n.T("add.preview.auth")+":", p.Auth)
	switch p.Auth {
	case store.AuthPassword:
		if p.Password != "" {
			fmt.Printf("  %-11s %s\n", i18n.T("add.preview.password")+":", i18n.T("add.preview.set"))
		}
	case store.AuthKey:
		fmt.Printf("  %-11s %s\n", i18n.T("add.preview.keypath")+":", p.KeyPath)
		if p.KeyPassphrase != "" {
			fmt.Printf("  %-11s %s\n", i18n.T("add.preview.passphrase")+":", i18n.T("add.preview.set"))
		}
	case store.AuthAgent:
		fmt.Println("  " + i18n.T("add.preview.agent"))
	}
	if p.Proxy != "" {
		fmt.Printf("  %-11s %s\n", i18n.T("add.preview.proxy")+":", p.Proxy)
	}
	if len(p.Options) > 0 {
		fmt.Println("  " + i18n.T("add.preview.options") + ":")
		for k, v := range p.Options {
			fmt.Printf("    %s = %s\n", k, v)
		}
	}
	if len(p.Tags) > 0 {
		fmt.Printf("  %-11s %s\n", i18n.T("add.preview.tags")+":", strings.Join(p.Tags, ", "))
	}
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()
}
