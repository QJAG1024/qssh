// Package keyring manages the 32-byte AES master key for the credential store.
//
// Safety rules:
//   - NEVER generate a new key when an encrypted store already exists.
//   - Cache the key after the first successful Get so load/save cannot diverge.
//   - Never overwrite an existing keyring secret with a freshly generated key.
package keyring

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Backend selects the key storage backend.
type Backend string

const (
	BackendKeyring Backend = "keyring"
	BackendFile    Backend = "file"
)

// Keyring manages the 32-byte AES encryption master key.
// Uses GNOME keyring (secret-tool) when backend is BackendKeyring,
// otherwise uses a file-based key at fallbackPath.
type Keyring struct {
	fallbackPath  string
	backend       Backend
	useSecretTool bool

	mu          sync.Mutex
	cachedKey   []byte // set after first successful Get; never regenerate mid-process
	storeExists bool   // when true, refuse to generate a new key
}

// New creates a Keyring with the given backend.
//   - BackendKeyring: uses secret-tool (GNOME Keyring) when available.
//   - BackendFile: uses a file at fallbackPath.
func New(fallbackPath string, backend Backend) *Keyring {
	kr := &Keyring{fallbackPath: fallbackPath, backend: backend}
	if backend == BackendKeyring {
		_, err := exec.LookPath("secret-tool")
		kr.useSecretTool = err == nil
	} else {
		// File backend may still migrate from secret-tool if present.
		_, err := exec.LookPath("secret-tool")
		kr.useSecretTool = err == nil
	}
	return kr
}

// SetStoreExists tells the keyring whether an encrypted store file already
// exists. Must be called before Get when opening an existing store so that a
// locked/missing keyring cannot silently mint a new master key.
func (k *Keyring) SetStoreExists(exists bool) {
	k.mu.Lock()
	k.storeExists = exists
	k.mu.Unlock()
}

// Get retrieves the 32-byte encryption key.
// The result is cached for the lifetime of this Keyring so load and save
// always use the same key.
func (k *Keyring) Get() ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.cachedKey != nil {
		out := make([]byte, len(k.cachedKey))
		copy(out, k.cachedKey)
		return out, nil
	}

	var key []byte
	var err error
	switch k.backend {
	case BackendKeyring:
		key, err = k.getWithKeyring()
	default:
		key, err = k.getWithFile()
	}
	if err != nil {
		return nil, err
	}
	k.cachedKey = make([]byte, len(key))
	copy(k.cachedKey, key)
	out := make([]byte, len(key))
	copy(out, key)
	return out, nil
}

// getWithKeyring uses secret-tool, with file-based migration fallback.
// Caller holds mu.
func (k *Keyring) getWithKeyring() ([]byte, error) {
	if !k.useSecretTool {
		// Config says keyring but binary is missing — try file, never generate if store exists.
		key, err := k.getFromFile()
		if err == nil {
			return key, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		if k.storeExists {
			return nil, fmt.Errorf("secret-tool not available and no store.key found, but encrypted store exists (set store.backend to \"file\" after restoring store.key, or install secret-tool)")
		}
		return k.mintNewKey(true)
	}

	key, err := k.getFromSecretTool()
	if err == nil {
		// Mirror to store.key so a locked keyring after reboot still has a recovery path.
		_ = k.setInFile(key)
		return key, nil
	}
	// secret-tool failed (locked, empty, or missing entry).
	// Prefer existing file key over minting a new one.
	fileKey, ferr := k.getFromFile()
	if ferr == nil {
		// Migrate file → keyring only if keyring accepts writes (unlocked).
		// Do NOT treat migration failure as fatal.
		_ = k.setInSecretTool(fileKey)
		return fileKey, nil
	}
	if !os.IsNotExist(ferr) {
		return nil, fmt.Errorf("read key file: %w", ferr)
	}

	// No key in keyring or file.
	if k.storeExists {
		return nil, fmt.Errorf("encryption key not available (login keyring locked or missing entry) and no store.key found, but encrypted store exists.\n"+
			"Unlock your session keyring (e.g. log into a desktop session / run: secret-tool search service qssh),\n"+
			"or restore ~/.config/qssh/store.key. Refusing to generate a new key to avoid data loss.\n"+
			"secret-tool error: %v", err)
	}
	return k.mintNewKey(true)
}

// getWithFile uses a file, with secret-tool migration fallback.
// Caller holds mu.
func (k *Keyring) getWithFile() ([]byte, error) {
	key, err := k.getFromFile()
	if err == nil {
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// Not in file — try secret-tool (migration from keyring).
	if k.useSecretTool {
		key, err = k.getFromSecretTool()
		if err == nil {
			// Persist to file for next time (best effort).
			_ = k.setInFile(key)
			return key, nil
		}
	}

	if k.storeExists {
		msg := "encryption key not found in store.key"
		if k.useSecretTool {
			msg += " (and secret-tool unavailable or empty)"
		}
		return nil, fmt.Errorf("%s, but encrypted store exists. Restore store.key or unlock keyring. Refusing to generate a new key", msg)
	}
	return k.mintNewKey(false)
}

// mintNewKey generates and persists a brand-new master key.
// Only called when no encrypted store exists yet.
// preferKeyring controls primary storage for BackendKeyring first-run.
// Caller holds mu.
func (k *Keyring) mintNewKey(preferKeyring bool) ([]byte, error) {
	key, err := k.generate()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if preferKeyring && k.useSecretTool {
		if err := k.setInSecretTool(key); err != nil {
			// Keyring unreachable — fall back to file for a brand-new store.
			if ferr := k.setInFile(key); ferr != nil {
				return nil, fmt.Errorf("store key: secret-tool: %w (file fallback: %v)", err, ferr)
			}
			return key, nil
		}
		// Also mirror to file as recovery aid (best effort).
		_ = k.setInFile(key)
		return key, nil
	}
	if err := k.setInFile(key); err != nil {
		return nil, fmt.Errorf("write key file: %w", err)
	}
	return key, nil
}

// generate creates a new random 32-byte key (in memory only).
func (k *Keyring) generate() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// getFromSecretTool retrieves the key via secret-tool.
func (k *Keyring) getFromSecretTool() ([]byte, error) {
	cmd := exec.Command("secret-tool", "lookup", "service", "qssh", "key", "store-key")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("secret-tool lookup failed: %w", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, fmt.Errorf("keyring returned empty key")
	}
	return hex.DecodeString(trimmed)
}

// setInSecretTool stores the key via secret-tool.
// WARNING: secret-tool store replaces any existing secret with the same attributes.
// Callers must only invoke this for migration of a known-good key or first mint.
func (k *Keyring) setInSecretTool(key []byte) error {
	encoded := hex.EncodeToString(key)
	cmd := exec.Command("secret-tool", "store", "--label=QSSH encryption key",
		"service", "qssh", "key", "store-key")
	cmd.Stdin = strings.NewReader(encoded)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret-tool store failed: %s: %w", string(out), err)
	}
	return nil
}

// getFromFile reads the key from the fallback file.
func (k *Keyring) getFromFile() ([]byte, error) {
	data, err := os.ReadFile(k.fallbackPath)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, fmt.Errorf("key file is empty")
	}
	return hex.DecodeString(trimmed)
}

// setInFile writes the key to the fallback file with 0600 permissions.
// Uses temp+rename so a crash cannot leave a truncated key file.
func (k *Keyring) setInFile(key []byte) error {
	dir := filepath.Dir(k.fallbackPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encoded := hex.EncodeToString(key) + "\n"
	tmp, err := os.CreateTemp(dir, ".store-key-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(encoded); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, k.fallbackPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
