package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"qssh/internal"
	"qssh/internal/i18n"
)

// Config handles --config get/set/unset operations.
// When no arguments are given and stdin is a TTY, opens an interactive panel.
func Config(args []string) {
	if len(args) == 0 {
		configInteractive()
		return
	}

	switch args[0] {
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, i18n.T("config.usage.get"))
			os.Exit(1)
		}
		getConfig(args[1])
	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, i18n.T("config.usage.set"))
			os.Exit(1)
		}
		setConfig(args[1], args[2])
	case "unset":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, i18n.T("config.usage.unset"))
			os.Exit(1)
		}
		unsetConfig(args[1])
	default:
		fmt.Fprintf(os.Stderr, i18n.T("config.unknown_action")+"\n", args[0])
		os.Exit(1)
	}
}

// configInteractive opens an interactive config panel.
func configInteractive() {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	if err := c.LoadError(); err != nil {
		fmt.Fprintf(os.Stderr, "config corrupt: %v\n", err)
		os.Exit(1)
	}
	all := c.All()

	knownKeys := configKnownKeys()
	// Merge known keys with actual values (some may be set but unknown).
	for k := range all {
		if _, ok := knownKeys[k]; !ok {
			knownKeys[k] = configKeyMeta{key: k, desc: ""}
		}
	}
	keys := make([]string, 0, len(knownKeys))
	for k := range knownKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for {
		fmt.Println()
		fmt.Println(strings.Repeat("─", 50))
		fmt.Println("  QSSH Configuration")
		fmt.Println(strings.Repeat("─", 50))

		for i, k := range keys {
			meta := knownKeys[k]
			val := all[k]
			display := val
			if display == "" {
				display = "(not set)"
			}
			desc := ""
			if meta.desc != "" {
				desc = "  ← " + meta.desc
			}
			fmt.Printf("  %2d) %-25s = %s%s\n", i+1, k, display, desc)
		}
		fmt.Println(strings.Repeat("─", 50))
		fmt.Println("  s) set a key")
		fmt.Println("  u) unset a key")
		fmt.Println("  q) quit")
		fmt.Println()

		choice := internal.Prompt("Action", "q")
		switch strings.ToLower(choice) {
		case "q", "":
			return
		case "s":
			setConfigInteractive(c, knownKeys)
			// Refresh
			all = c.All()
		case "u":
			unsetConfigInteractive(c)
			all = c.All()
		default:
			// Try numeric to jump to a key
			n, err := fmt.Sscanf(choice, "%d")
			if err == nil && n == 1 {
				var idx int
				fmt.Sscanf(choice, "%d", &idx)
				if idx >= 1 && idx <= len(keys) {
					k := keys[idx-1]
					meta := knownKeys[k]
					editConfigKey(c, k, meta)
					all = c.All()
				}
			}
		}
	}
}

func setConfigInteractive(c *internal.Config, known map[string]configKeyMeta) {
	fmt.Println()
	for _, k := range sortedKeys(known) {
		meta := known[k]
		fmt.Printf("  %s", k)
		if len(meta.options) > 0 {
			fmt.Printf("  [%s]", strings.Join(meta.options, "|"))
		}
		fmt.Println()
	}
	fmt.Println()

	key := internal.Prompt("Key", "")
	if key == "" {
		return
	}
	meta, ok := known[key]
	if !ok {
		meta = configKeyMeta{key: key}
	}
	editConfigKey(c, key, meta)
}

func editConfigKey(c *internal.Config, key string, meta configKeyMeta) {
	val := c.Get(key)
	if len(meta.options) > 0 {
		val = internal.SelectPrompt("Value:", meta.options, val)
	} else {
		defaultVal := val
		if defaultVal == "" {
			defaultVal = meta.defaultVal
		}
		val = internal.Prompt("Value", defaultVal)
	}
	if val == "" {
		// User wants to unset
		if internal.Confirm("Unset this key?", false) {
			if err := c.Set(key, ""); err != nil {
				fmt.Fprintf(os.Stderr, i18n.T("config.save_error")+"\n", err)
				return
			}
			fmt.Printf(i18n.T("config.unset")+"\n", key)
			return
		}
		return
	}
	if err := c.Set(key, val); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("config.save_error")+"\n", err)
		return
	}
	fmt.Printf(i18n.T("config.set")+"\n", key, val)
}

func unsetConfigInteractive(c *internal.Config) {
	all := c.All()
	if len(all) == 0 {
		fmt.Println("No keys to unset")
		return
	}
	keys := sortedKeys2(all)
	key := internal.SelectPrompt("Unset which key?", keys, "")
	if key == "" {
		return
	}
	if err := c.Set(key, ""); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("config.save_error")+"\n", err)
		return
	}
	fmt.Printf(i18n.T("config.unset")+"\n", key)
}

// configKeyMeta describes a known config key.
type configKeyMeta struct {
	key        string
	desc       string
	defaultVal string
	options    []string
}

// configKnownKeys returns the list of known config keys with metadata.
func configKnownKeys() map[string]configKeyMeta {
	return map[string]configKeyMeta{
		"hostkey.mode": {
			desc:    "Host key verification policy",
			options: []string{"tofu", "strict"},
		},
		"lang": {
			desc:    "UI language (requires restart)",
			options: []string{"en-US", "zh-CN"},
		},
		"store.backend": {
			desc:    "Encryption key storage backend",
			options: []string{"file", "keyring"},
		},
		"sftp.allow_non_loopback": {
			desc:    "Allow SFTP proxy to bind non-loopback",
			options: []string{"false", "true"},
		},
		"term.mode": {
			desc: "PTY $TERM passthrough (compat=force xterm)",
		},
	}
}

func sortedKeys(m map[string]configKeyMeta) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys2(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func listConfig() {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	if err := c.LoadError(); err != nil {
		fmt.Fprintf(os.Stderr, "config corrupt: %v\n", err)
		os.Exit(1)
	}
	all := c.All()
	if len(all) == 0 {
		fmt.Println(i18n.T("config.empty"))
		return
	}
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s = %s\n", k, all[k])
	}
}

func getConfig(key string) {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	if err := c.LoadError(); err != nil {
		fmt.Fprintf(os.Stderr, "config corrupt: %v\n", err)
		os.Exit(1)
	}
	val := c.Get(key)
	if val == "" {
		fmt.Println(i18n.T("config.not_set"))
	} else {
		fmt.Println(val)
	}
}

func setConfig(key, value string) {
	if err := validateConfigValue(key, value); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	c := internal.OpenConfig(internal.DefaultConfigPath())
	if err := c.LoadError(); err != nil {
		fmt.Fprintf(os.Stderr, "config corrupt: %v\n", err)
		os.Exit(1)
	}
	if err := c.Set(key, value); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("config.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s = %s\n", key, value)
}

// validateConfigValue rejects security-sensitive keys with unknown values
// so a typo cannot silently weaken policy later at connection time.
func validateConfigValue(key, value string) error {
	switch key {
	case "hostkey.mode":
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "tofu", "strict":
			return nil
		default:
			return fmt.Errorf("invalid hostkey.mode %q (supported: tofu, strict)", value)
		}
	case "sftp.allow_non_loopback":
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "false", "1", "0", "yes", "no":
			return nil
		default:
			return fmt.Errorf("invalid sftp.allow_non_loopback %q (supported: true, false)", value)
		}
	}
	return nil
}

func unsetConfig(key string) {
	c := internal.OpenConfig(internal.DefaultConfigPath())
	if err := c.Set(key, ""); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("config.save_error")+"\n", err)
		os.Exit(1)
	}
	fmt.Printf(i18n.T("config.unset")+"\n", key)
}
