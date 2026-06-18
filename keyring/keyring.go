package keyring

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
}

// New creates a Keyring with the given backend.
//   - BackendKeyring: uses secret-tool (GNOME Keyring); errors if not found.
//   - BackendFile: uses a file at fallbackPath.
func New(fallbackPath string, backend Backend) *Keyring {
	kr := &Keyring{fallbackPath: fallbackPath, backend: backend}
	if backend == BackendKeyring {
		_, err := exec.LookPath("secret-tool")
		kr.useSecretTool = err == nil
	}
	return kr
}

// Get retrieves the 32-byte encryption key.
// If the active backend doesn't have one yet, it tries to migrate from the
// other backend before generating a fresh key.
func (k *Keyring) Get() ([]byte, error) {
	switch k.backend {
	case BackendKeyring:
		return k.getWithKeyring()
	default:
		return k.getWithFile()
	}
}

// getWithKeyring uses secret-tool, with file-based migration fallback.
func (k *Keyring) getWithKeyring() ([]byte, error) {
	if !k.useSecretTool {
		return nil, fmt.Errorf("secret-tool not available (set store.backend to \"file\" to use file-based key storage)")
	}

	key, err := k.getFromSecretTool()
	if err == nil {
		return key, nil
	}

	// Not in keyring — try file (migration from old file-based setup).
	key, err = k.getFromFile()
	if err == nil {
		// Store into keyring for next time.
		if setErr := k.setInSecretTool(key); setErr == nil {
			return key, nil
		}
		// keyring broken despite probe passing? keep using file silently.
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// No key anywhere — generate fresh.
	key, err = k.generate()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if err := k.setInSecretTool(key); err != nil {
		// keyring daemon unreachable — fall back to file.
		if ferr := k.setInFile(key); ferr != nil {
			return nil, fmt.Errorf("store key: secret-tool: %w (file fallback: %v)", err, ferr)
		}
		return key, nil
	}
	return key, nil
}

// getWithFile uses a file, with secret-tool migration fallback.
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
			if setErr := k.setInFile(key); setErr == nil {
				return key, nil
			}
			return key, nil
		}
	}

	// No key anywhere — generate fresh.
	key, err = k.generate()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if err := k.setInFile(key); err != nil {
		return nil, fmt.Errorf("write key file: %w", err)
	}
	return key, nil
}

// generate creates a new random 32-byte key.
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
	return hex.DecodeString(trimmed)
}

// setInFile writes the key to the fallback file with 0600 permissions.
func (k *Keyring) setInFile(key []byte) error {
	dir := filepath.Dir(k.fallbackPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encoded := hex.EncodeToString(key)
	return os.WriteFile(k.fallbackPath, []byte(encoded+"\n"), 0600)
}