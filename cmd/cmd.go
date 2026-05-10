package cmd

import (
	"os"
	"path/filepath"

	"qssh/internal"
	"qssh/keyring"
	"qssh/store"
)

// defaultStorePath returns the default path for the encrypted store file.
func defaultStorePath() string {
	if p := os.Getenv("QSSH_STORE_PATH"); p != "" {
		return p
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "store.json")
}

// defaultKeyPath returns the default fallback path for the encryption key file.
func defaultKeyPath() string {
	if p := os.Getenv("QSSH_KEY_PATH"); p != "" {
		return p
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "store.key")
}

// openStore creates a Store ready for use.
func openStore() (*store.Store, error) {
	kr := keyring.New(defaultKeyPath())
	return store.New(defaultStorePath(), kr)
}

// progressOutput wraps internal.RenderProgress.
var progressOutput internal.ProgressFn = internal.RenderProgress

// authMethodOptions returns the supported auth methods as a comma-separated list.
func authMethodOptions() string {
	return "password, key, agent, keyboard-interactive"
}

// validAuthMethods returns the list of valid auth methods for prompting.
var validAuthMethods = []string{"password", "key", "agent", "keyboard-interactive"}