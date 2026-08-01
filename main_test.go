package main

import "testing"

func TestArgvCredsPresent(t *testing.T) {
	cases := []struct {
		name          string
		password      string
		keyPassphrase string
		want          bool
	}{
		{"no creds", "", "", false},
		{"password only", "secret", "", true},
		{"keypass only", "", "passphrase", true},
		{"both", "secret", "passphrase", true},
		{"empty strings treated as absent", " ", "  ", true}, // whitespace still counts (non-empty)
	}
	for _, c := range cases {
		if got := argvCredsPresent(c.password, c.keyPassphrase); got != c.want {
			t.Errorf("%s: argvCredsPresent(%q, %q) = %v, want %v",
				c.name, c.password, c.keyPassphrase, got, c.want)
		}
	}
}
