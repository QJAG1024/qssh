package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config manages persistent key-value settings.
// Mutations take a cross-process file lock so concurrent
// `qssh --config set` invocations cannot lose updates.
type Config struct {
	mu      sync.Mutex
	path    string
	data    map[string]string
	loadErr error // non-nil when the on-disk file exists but could not be parsed
}

// DefaultConfigPath returns the default config file location.
func DefaultConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "config.json")
}

// OpenConfig loads the config file. Missing file yields an empty config.
// A present-but-corrupt file sets LoadError() so security-sensitive
// readers can fail closed instead of silently using defaults.
func OpenConfig(path string) *Config {
	c := &Config{path: path, data: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			c.loadErr = fmt.Errorf("read config: %w", err)
		}
		return c
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		c.loadErr = fmt.Errorf("parse config %s: %w", path, err)
		// Keep data empty — callers that ignore LoadError still see no keys,
		// but security paths must check LoadError first.
		return c
	}
	if m == nil {
		m = map[string]string{}
	}
	c.data = m
	return c
}

// LoadError returns a non-nil error when the config file exists but could
// not be loaded. Callers that make security decisions from config MUST
// check this (fail closed) rather than treating missing keys as defaults.
func (c *Config) LoadError() error {
	if c == nil {
		return nil
	}
	return c.loadErr
}

// Get returns a config value by key, or empty string if not set.
func (c *Config) Get(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data[key]
}

// Set updates a config value and persists to disk under a process lock.
// Refuses to overwrite a corrupt file unless the write is the first
// successful recovery (caller should surface LoadError first).
func (c *Config) Set(key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loadErr != nil {
		return fmt.Errorf("config is corrupt, refuse to write: %w", c.loadErr)
	}

	fl, err := Lock(c.path)
	if err != nil {
		return fmt.Errorf("lock config: %w", err)
	}
	defer fl.Unlock()

	// Re-read under lock so concurrent writers don't clobber each other.
	if err := c.reloadLocked(); err != nil {
		return err
	}
	if value == "" {
		delete(c.data, key)
	} else {
		c.data[key] = value
	}
	return c.saveLocked()
}

func (c *Config) reloadLocked() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			if c.data == nil {
				c.data = map[string]string{}
			}
			c.loadErr = nil
			return nil
		}
		return err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if m == nil {
		m = map[string]string{}
	}
	c.data = m
	c.loadErr = nil
	return nil
}

func (c *Config) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: temp + rename, same directory.
	dir := filepath.Dir(c.path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
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
	if _, err := tmp.Write(data); err != nil {
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
	if err := os.Rename(tmpName, c.path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// All returns a copy of all config entries.
func (c *Config) All() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := make(map[string]string, len(c.data))
	for k, v := range c.data {
		m[k] = v
	}
	return m
}
