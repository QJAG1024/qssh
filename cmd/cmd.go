package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
// On first run it probes which key backend is usable and persists the choice.
// Prefer file backend when store.key already exists and is readable — more
// reliable across reboots than a session-locked GNOME keyring.
func openStore() (*store.Store, error) {
	cfg := internal.OpenConfig(internal.DefaultConfigPath())

	backendStr := cfg.Get("store.backend")
	if backendStr == "" {
		backendStr = probeBackend()
		// Best-effort persist. If config is corrupt, still open the store
		// using the probed backend — security-sensitive readers (hostkey.mode)
		// check LoadError() separately and fail closed.
		if cfg.LoadError() == nil {
			if err := cfg.Set("store.backend", backendStr); err != nil {
				return nil, fmt.Errorf("persist backend config: %w", err)
			}
		}
	}

	// If config prefers keyring but a working store.key is present and keyring
	// is locked, Store.New will fail with a clear error (no silent re-key).
	// Operators can: unlock keyring, or `qssh --config set store.backend file`.
	kr := keyring.New(defaultKeyPath(), keyring.Backend(backendStr))
	return store.New(defaultStorePath(), kr)
}

// probeBackend detects which key backend is usable for a first-time install.
// Prefer file when store.key already exists (reboot-safe). Prefer keyring only
// when secret-tool works AND no file key is present yet.
func probeBackend() string {
	// Existing file key → file backend (stable across reboots).
	if _, err := os.Stat(defaultKeyPath()); err == nil {
		return string(keyring.BackendFile)
	}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return string(keyring.BackendFile)
	}
	// secret-tool exists — test if the keyring daemon is unlocked
	// by attempting to store and immediately remove a probe entry.
	probeOk := false
	storeCmd := exec.Command("secret-tool", "store",
		"--label=qssh-probe",
		"service", "qssh-probe",
		"key", "probe")
	storeCmd.Stdin = strings.NewReader("probe")
	if storeCmd.Run() == nil {
		probeOk = true
		exec.Command("secret-tool", "clear",
			"service", "qssh-probe",
			"key", "probe").Run()
	}
	if probeOk {
		return string(keyring.BackendKeyring)
	}
	return string(keyring.BackendFile)
}

// progressOutput wraps internal.RenderProgress.
var progressOutput internal.ProgressFn = internal.RenderProgress

// authMethodOptions returns the supported auth methods as a comma-separated list.
func authMethodOptions() string {
	return "password, key, agent, keyboard-interactive"
}

// validAuthMethods returns the list of valid auth methods for prompting.
var validAuthMethods = []string{"password", "key", "agent", "keyboard-interactive"}
