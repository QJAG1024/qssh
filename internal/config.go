package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config manages persistent key-value settings.
type Config struct {
	mu   sync.Mutex
	path string
	data map[string]string
}

// DefaultConfigPath returns the default config file location.
func DefaultConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "config.json")
}

// OpenConfig loads the config file (creates if missing).
func OpenConfig(path string) *Config {
	c := &Config{path: path, data: map[string]string{}}
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &c.data)
	}
	if c.data == nil {
		c.data = map[string]string{}
	}
	return c
}

// Get returns a config value by key, or empty string if not set.
func (c *Config) Get(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data[key]
}

// Set updates a config value and persists to disk.
func (c *Config) Set(key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if value == "" {
		delete(c.data, key)
	} else {
		c.data[key] = value
	}
	return c.save()
}

func (c *Config) save() error {
	os.MkdirAll(filepath.Dir(c.path), 0700)
	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600)
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