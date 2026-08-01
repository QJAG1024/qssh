package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/store"
)

// ExportOpts controls qssh --export.
type ExportOpts struct {
	Name string // profile to export (required)
	Dir  string // output dir; empty = interactive prompt, default pwd
}

// Export exports a single profile to an encrypted .qssh file.
// Non-interactive when Dir is set: the passphrase is read from stdin.
func Export(name, dir string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
		os.Exit(1)
	}
	p, ok := s.Get(name)
	if !ok {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
		os.Exit(1)
	}

	// Read the private key file if present (embedded so cross-machine imports
	// can restore it). Missing/unreadable key files are not an error — the
	// export still carries key_path as a reference.
	var keyData []byte
	if p.KeyPath != "" {
		if data, err := os.ReadFile(internal.ExpandPath(p.KeyPath)); err == nil {
			keyData = data
		}
	}

	// Resolve output directory: --dir wins; interactive asks; default pwd.
	outDir := dir
	if outDir == "" {
		if internal.IsTerminalStdin() {
			outDir = internal.Prompt(i18n.T("export.dir"), ".")
		} else {
			outDir = "."
		}
	}
	if fi, err := os.Stat(outDir); err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, i18n.T("export.dir_not_found")+"\n", outDir)
		os.Exit(1)
	}
	outPath := filepath.Join(outDir, name+".qssh")

	// Passphrase: non-interactive (--dir) reads stdin; interactive asks twice.
	var passphrase string
	if dir != "" {
		passphrase = readStdinPassphrase()
	} else {
		var err error
		passphrase, err = internal.ReadPasswordWithConfirm(i18n.T("export.passphrase"))
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
			os.Exit(1)
		}
	}
	if passphrase == "" {
		fmt.Fprintln(os.Stderr, i18n.T("export.empty_passphrase"))
		os.Exit(1)
	}

	data, err := store.ExportProfile(p, passphrase, keyData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "export: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "export: %v\n", err)
		os.Exit(1)
	}

	// Proxy was dropped by design — tell the user.
	if p.Proxy != "" {
		fmt.Fprintln(os.Stderr, i18n.T("export.proxy_dropped"))
	}
	fmt.Printf(i18n.T("export.done")+"\n", outPath)
}

// ImportOpts controls qssh --import.
type ImportOpts struct {
	File string // path to .qssh file (required)
	Name string // profile name; empty = interactive prompt
}

// Import restores a profile from an encrypted .qssh file.
// Non-interactive when Name is set: the passphrase is read from stdin.
func Import(file, name string) {
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import: %v\n", err)
		os.Exit(1)
	}
	if !store.IsExportFile(data) {
		fmt.Fprintln(os.Stderr, i18n.T("import.not_export"))
		os.Exit(1)
	}

	// Passphrase: non-interactive (--name) reads stdin; interactive asks once.
	var passphrase string
	if name != "" {
		passphrase = readStdinPassphrase()
	} else {
		var err error
		passphrase, err = internal.ReadPassword(i18n.T("import.passphrase"))
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("password.read_error")+"\n", err)
			os.Exit(1)
		}
	}
	if passphrase == "" {
		fmt.Fprintln(os.Stderr, i18n.T("export.empty_passphrase"))
		os.Exit(1)
	}

	payload, err := store.ImportProfile(data, passphrase)
	if err != nil {
		// Distinguish wrong-passphrase/corrupt from unsupported-version.
		if strings.Contains(err.Error(), "wrong passphrase") {
			fmt.Fprintln(os.Stderr, i18n.T("import.wrong_passphrase"))
		} else if strings.Contains(err.Error(), "unsupported export version") {
			fmt.Fprintln(os.Stderr, i18n.T("import.version_mismatch"))
		} else {
			fmt.Fprintf(os.Stderr, "import: %v\n", err)
		}
		os.Exit(1)
	}

	// Name: --name wins; interactive asks (defaults to basename of payload
	// host, validated against profile-name rules).
	profileName := name
	if profileName == "" {
		defaultName := sanitizeHostToName(payload.Host)
		profileName = internal.Prompt(i18n.T("import.name"), defaultName)
	}
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
		os.Exit(1)
	}
	if _, exists := s.Get(profileName); exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.exists")+"\n", profileName)
		os.Exit(1)
	}

	// Restore the private key if it was embedded and the target path is
	// missing. Interactive asks where to write; non-interactive falls back to
	// ~/.ssh/<basename> and reports the location.
	if len(payload.KeyData) > 0 && payload.KeyPath != "" {
		finalPath, err := restoreKey(payload.KeyPath, payload.KeyData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "import: %v\n", err)
			os.Exit(1)
		}
		payload.KeyPath = finalPath
	}

	p := store.Profile{
		Name:          profileName,
		Host:          payload.Host,
		Port:          payload.Port,
		User:          payload.User,
		Auth:          payload.Auth,
		Password:      payload.Password,
		KeyPath:       payload.KeyPath,
		KeyPassphrase: payload.KeyPassphrase,
		Options:       payload.Options,
		Tags:          payload.Tags,
	}
	if err := s.Add(p); err != nil {
		fmt.Fprintf(os.Stderr, "import: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf(i18n.T("profile.created")+"\n", profileName, profileName)
}

// restoreKey writes embedded key data to keyPath if that file does not exist.
// Returns the final key path (updated if redirected to ~/.ssh) or an error.
// Non-interactive imports use the default ~/.ssh/<basename> when keyPath's
// directory is unusable, and print where the key landed.
func restoreKey(keyPath string, keyData []byte) (string, error) {
	full := internal.ExpandPath(keyPath)
	if _, err := os.Stat(full); err == nil {
		return keyPath, nil // local key already present; keep key_path as-is
	}

	// Try writing to the original path first (its dir may exist and be
	// writable). On any failure — missing dir, permission, relative path
	// resolving to an unwritable CWD — fall back to ~/.ssh/<basename>.
	if err := os.WriteFile(full, keyData, 0600); err == nil {
		return full, nil
	}

	home, herr := os.UserHomeDir()
	if herr != nil {
		return keyPath, herr
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return keyPath, err
	}
	fallback := filepath.Join(sshDir, filepath.Base(keyPath))
	if err := os.WriteFile(fallback, keyData, 0600); err != nil {
		return keyPath, err
	}
	fmt.Fprintln(os.Stderr, i18n.T("import.key_redirected"), fallback)
	return fallback, nil
}

// sanitizeHostToName turns a host into a valid profile name (letters, digits,
// dash, underscore, dot). IPv6 brackets are dropped, colons/slashes become a
// dash, consecutive dashes collapse, and leading/trailing dashes are trimmed.
func sanitizeHostToName(host string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '_', r == '.':
			b.WriteRune(r)
			lastDash = false
		case r == ':' || r == '/' || r == '[' || r == ']':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "imported"
	}
	return out
}

// readStdinPassphrase reads one line of passphrase from stdin (used by
// non-interactive --export --dir and --import --name).
func readStdinPassphrase() string {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimSuffix(line, "\n")
}
