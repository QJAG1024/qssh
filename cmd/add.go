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
	fmt.Printf("  Creating profile: %s\n", p.Name)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	// Host
	p.Host = internal.Prompt("Host", "")
	if p.Host == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_host"))
		os.Exit(1)
	}

	// Port
	portStr := internal.Prompt("Port", "22")
	p.Port, _ = strconv.Atoi(portStr)

	// User
	// User — default to current OS user
	defaultUser := os.Getenv("USER")
	if defaultUser == "" {
		defaultUser = os.Getenv("USERNAME")
	}
	p.User = internal.Prompt("User", defaultUser)
	if p.User == "" {
		fmt.Fprintln(os.Stderr, i18n.T("field.required_user"))
		os.Exit(1)
	}

	// Auth method — numbered selection
	authMethods := validAuthMethods
	authStr := internal.SelectPrompt("Auth method:", authMethods, "password")
	switch strings.ToLower(authStr) {
	case "password", "p":
		p.Auth = store.AuthPassword
		pass, err := internal.ReadPasswordWithConfirm("Password")
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
			os.Exit(1)
		}
		p.Password = pass
	case "key", "k":
		p.Auth = store.AuthKey
		p.KeyPath = internal.Prompt("Key path", "~/.ssh/id_ed25519")
		p.KeyPath = internal.ExpandPath(p.KeyPath)
		// Verify key file exists
		if _, err := os.Stat(p.KeyPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: key file %q not found: %v\n", p.KeyPath, err)
		}
		if internal.Confirm("Key has passphrase?", false) {
			pass, err := internal.ReadPassword("Key passphrase")
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
	proxy := internal.Prompt("ProxyJump profile (optional)", "")
	if proxy != "" {
		// Validate proxy profile exists
		if _, exists := s.Get(proxy); !exists {
			fmt.Fprintf(os.Stderr, "Warning: proxy profile %q not found, will be created later\n", proxy)
		}
		// Cycle detection: proxy cannot point to self
		if proxy == p.Name {
			fmt.Fprintln(os.Stderr, "Error: proxy cannot point to the same profile")
			os.Exit(1)
		}
		p.Proxy = proxy
	}

	// Options
	optStr := internal.Prompt("Options (comma-separated KEY=VALUE, optional)", "")
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
	tagsStr := internal.Prompt("Tags (comma-separated, optional)", "")
	if tagsStr != "" {
		p.Tags = parseTags(tagsStr)
	}

	// Preview summary
	printSummary(*p)

	if !internal.Confirm("Save profile?", true) {
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
	fmt.Println("  Preview")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Printf("  Name:      %s\n", p.Name)
	fmt.Printf("  Host:      %s:%d\n", p.Host, p.Port)
	fmt.Printf("  User:      %s\n", p.User)
	fmt.Printf("  Auth:      %s\n", p.Auth)
	switch p.Auth {
	case store.AuthPassword:
		if p.Password != "" {
			fmt.Println("  Password:  (set)")
		}
	case store.AuthKey:
		fmt.Printf("  Key path:  %s\n", p.KeyPath)
		if p.KeyPassphrase != "" {
			fmt.Println("  Passphrase: (set)")
		}
	case store.AuthAgent:
		fmt.Println("  Agent:     (SSH agent)")
	}
	if p.Proxy != "" {
		fmt.Printf("  Proxy:     %s\n", p.Proxy)
	}
	if len(p.Options) > 0 {
		fmt.Println("  Options:")
		for k, v := range p.Options {
			fmt.Printf("    %s = %s\n", k, v)
		}
	}
	if len(p.Tags) > 0 {
		fmt.Printf("  Tags:      %s\n", strings.Join(p.Tags, ", "))
	}
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()
}
