package store

import (
	"testing"
	"time"
)

func TestProfileValidate_Valid(t *testing.T) {
	p := Profile{
		Name: "test",
		Host: "192.168.1.1",
		Port: 22,
		User: "root",
		Auth: AuthPassword,
		Password: "secret",
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestProfileValidate_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		profile Profile
		wantErr string
	}{
		{"empty name", Profile{Host: "h", Port: 22, User: "u", Auth: "password", Password: "p"}, "name is required"},
		{"empty host", Profile{Name: "n", Port: 22, User: "u", Auth: "password", Password: "p"}, "host is required"},
		{"port zero", Profile{Name: "n", Host: "h", Port: 0, User: "u", Auth: "password", Password: "p"}, "port"},
		{"port too high", Profile{Name: "n", Host: "h", Port: 99999, User: "u", Auth: "password", Password: "p"}, "port"},
		{"empty user", Profile{Name: "n", Host: "h", Port: 22, User: "", Auth: "password", Password: "p"}, "user is required"},
		{"bad auth method", Profile{Name: "n", Host: "h", Port: 22, User: "u", Auth: "invalid", Password: "p"}, "unsupported"},
		{"password auth no password", Profile{Name: "n", Host: "h", Port: 22, User: "u", Auth: "password"}, "password is required"},
		{"key auth no key path", Profile{Name: "n", Host: "h", Port: 22, User: "u", Auth: "key"}, "key path is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestSetDefaults(t *testing.T) {
	p := Profile{Name: "n", Host: "h", User: "u"}
	p.SetDefaults()
	if p.Port != 22 {
		t.Errorf("expected port 22, got %d", p.Port)
	}
	if p.Auth != AuthPassword {
		t.Errorf("expected auth password, got %s", p.Auth)
	}
}

func TestSetDefaults_PreservesExplicit(t *testing.T) {
	p := Profile{Name: "n", Host: "h", User: "u", Port: 2222, Auth: AuthKey}
	p.SetDefaults()
	if p.Port != 2222 {
		t.Errorf("expected port 2222, got %d", p.Port)
	}
	if p.Auth != AuthKey {
		t.Errorf("expected auth key, got %s", p.Auth)
	}
}

func TestCopy(t *testing.T) {
	orig := Profile{
		Name:    "test",
		Host:    "h",
		Port:    22,
		User:    "u",
		Auth:    AuthPassword,
		Password: "secret",
		Tags:    []string{"prod", "web"},
		CreatedAt: time.Now(),
	}
	c := orig.Copy()
	// Modify original tags to ensure they're independent
	orig.Tags[0] = "changed"
	if c.Tags[0] != "prod" {
		t.Errorf("copy should be independent, got %s", c.Tags[0])
	}
}