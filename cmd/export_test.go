package cmd

import (
	"testing"
)

func TestSanitizeHostToName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"example.com", "example.com"},
		{"10.0.0.5", "10.0.0.5"},
		{"[::1]", "1"}, // brackets+colons collapse to single dash, trimmed
		{"host:2222", "host-2222"},
		{"a/b/c", "a-b-c"},
		{"", "imported"},
	}
	for _, c := range cases {
		if got := sanitizeHostToName(c.in); got != c.want {
			t.Errorf("sanitizeHostToName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
