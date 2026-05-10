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

// Keyring manages the 32-byte AES encryption master key.
// Uses GNOME keyring (secret-tool) when available,
// falls back to a file-based key at fallbackPath.
type Keyring struct {
	fallbackPath string
	useSecretTool bool
}

// New creates a Keyring. If secret-tool is found on $PATH,
// it uses the system keyring; otherwise falls back to a file.
func New(fallbackPath string) *Keyring {
	kr := &Keyring{fallbackPath: fallbackPath}
	if _, err := exec.LookPath("secret-tool"); err == nil {
		kr.useSecretTool = true
	}
	return kr
}

// Get retrieves the 32-byte encryption key.
// It tries keyring first, then fallback file.
// If neither exists, it generates a new key and stores it.
func (k *Keyring) Get() ([]byte, error) {
	if k.useSecretTool {
		key, err := k.getFromSecretTool()
		if err == nil {
			return key, nil
		}
		// Key not in keyring — generate and store
		key, err = k.generate()
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
		if err := k.setInSecretTool(key); err != nil {
			return nil, fmt.Errorf("store key in keyring: %w", err)
		}
		return key, nil
	}

	// File-based fallback
	key, err := k.getFromFile()
	if err == nil {
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	// File doesn't exist — generate and store
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