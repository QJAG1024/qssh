package store

import (
	"fmt"
	"time"
)

// AuthMethod represents an SSH authentication strategy.
type AuthMethod string

const (
	AuthPassword           AuthMethod = "password"
	AuthKey                AuthMethod = "key"
	AuthAgent              AuthMethod = "agent"
	AuthKeyboardInteractive AuthMethod = "keyboard-interactive"
)

// ValidAuthMethods contains all supported auth methods for validation.
var ValidAuthMethods = map[AuthMethod]bool{
	AuthPassword:           true,
	AuthKey:                true,
	AuthAgent:              true,
	AuthKeyboardInteractive: true,
}

// Profile represents a single SSH connection credential profile.
type Profile struct {
	Name      string     `json:"name"`
	Host      string     `json:"host"`
	Port      int        `json:"port"`
	User      string     `json:"user"`
	Auth      AuthMethod `json:"auth"`

	// Password is stored encrypted in the store (AES-256-GCM).
	Password string `json:"password,omitempty"`

	// Key-based auth fields.
	KeyPath       string `json:"key_path,omitempty"`
	KeyPassphrase string `json:"key_passphrase,omitempty"`

	// Proxy is the name of another profile to use as a jump host.
	// When set, the connection is tunneled through the proxy host.
	Proxy string `json:"proxy,omitempty"`

	// Options holds per-profile SSH options (ConnectTimeout, SetEnv, etc.).
	Options map[string]string `json:"options,omitempty"`

	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastUsed        time.Time `json:"last_used,omitempty"`
	ConnectionCount int       `json:"connection_count"`

	Tags []string `json:"tags,omitempty"`
}

// Validate checks that the profile has all required fields and valid values.
func (p *Profile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if p.Host == "" {
		return fmt.Errorf("host is required")
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if p.User == "" {
		return fmt.Errorf("user is required")
	}
	if !ValidAuthMethods[p.Auth] {
		return fmt.Errorf("unsupported auth method %q (supported: password, key, agent, keyboard-interactive)", p.Auth)
	}
	switch p.Auth {
	case AuthPassword:
		if p.Password == "" {
			return fmt.Errorf("password is required for password auth")
		}
	case AuthKey:
		if p.KeyPath == "" {
			return fmt.Errorf("key path is required for key auth")
		}
	}
	return nil
}

// SetDefaults fills in zero values with sensible defaults.
func (p *Profile) SetDefaults() {
	if p.Port == 0 {
		p.Port = 22
	}
	if p.Auth == "" {
		p.Auth = AuthPassword
	}
}

// Copy returns a deep copy of the profile.
func (p *Profile) Copy() Profile {
	c := *p
	if p.Tags != nil {
		c.Tags = make([]string, len(p.Tags))
		copy(c.Tags, p.Tags)
	}
	if p.Options != nil {
		c.Options = make(map[string]string, len(p.Options))
		for k, v := range p.Options {
			c.Options[k] = v
		}
	}
	return c
}