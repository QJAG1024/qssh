package cmd

import (
	"testing"
	"time"

	"qssh/store"
)

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"simple", "simple"},                       // safe chars pass through
		{"./script.sh", "./script.sh"},             // path chars safe
		{"has space", "'has space'"},               // space quoted
		{"a'b", `'a'\''b'`},                        // embedded quote escaped
		{"$(rm -rf /)", "'$(rm -rf /)'"},           // command substitution neutralized
		{"`backtick`", "'`backtick`'"},             // backticks neutralized
		{"; rm -rf /", "'; rm -rf /'"},             // semicolon neutralized
		{"a\nb", "'a\nb'"},                         // newline inside single quotes
		{"-n", "-n"},                               // dash ok
		{"file:1,2", "file:1,2"},                   // colon/comma safe
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"echo", "hello world", "$HOME"})
	if got != "echo 'hello world' '$HOME'" {
		t.Errorf("shellJoin = %q", got)
	}
}

func TestBuildRemoteCommand(t *testing.T) {
	// No args -> legacy Cmd string passthrough.
	if got := buildRemoteCommand(daemonReq{Cmd: "echo hi"}); got != "echo hi" {
		t.Errorf("legacy cmd = %q", got)
	}
	// Single arg -> treated as full shell command (preserves quoting).
	if got := buildRemoteCommand(daemonReq{Args: []string{"echo 'a b'"}}); got != "echo 'a b'" {
		t.Errorf("single arg = %q", got)
	}
	// Multiple args -> shell-quoted argv, no injection.
	got := buildRemoteCommand(daemonReq{Args: []string{"echo", "x; rm -rf /"}})
	if got != "echo 'x; rm -rf /'" {
		t.Errorf("multi arg = %q, want injection-neutralized", got)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{2 * time.Hour, "2h0m"},
		{3 * time.Hour, "3h0m"},
	}
	for _, c := range cases {
		if got := formatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestParseTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b ,c", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := parseTags(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseTags(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseTags(%q) = %v, want %v", c.in, got, c.want)
			}
		}
	}
}

func TestOptionStringAndParse(t *testing.T) {
	m := map[string]string{"ConnectTimeout": "30s", "SetEnv": "LANG=C"}
	s := optionString(m)
	if s != "ConnectTimeout=30s,SetEnv=LANG=C" && s != "SetEnv=LANG=C,ConnectTimeout=30s" {
		t.Errorf("optionString = %q", s)
	}
	parsed := parseOptionString("ConnectTimeout=30s,SetEnv=LANG=C, bad")
	if parsed["ConnectTimeout"] != "30s" || parsed["SetEnv"] != "LANG=C" {
		t.Errorf("parseOptionString = %v", parsed)
	}
	if _, ok := parsed["bad"]; ok {
		t.Error("malformed pair without '=' should be skipped")
	}
	if parseOptionString("") != nil {
		t.Error("empty string should return nil")
	}
}

func TestProfileIdentityEqual(t *testing.T) {
	base := store.Profile{Name: "a", Host: "h", Port: 22, User: "u", Auth: store.AuthPassword, Password: "p"}
	if !profileIdentityEqual(base, base) {
		t.Error("identical profiles should be equal")
	}
	// Credential change matters.
	diff := base
	diff.Password = "changed"
	if profileIdentityEqual(base, diff) {
		t.Error("password change should invalidate identity")
	}
	// Proxy link matters.
	diff = base
	diff.Proxy = "jump"
	if profileIdentityEqual(base, diff) {
		t.Error("proxy change should invalidate identity")
	}
	// Name does NOT matter (identity is connection-defining only).
	diff = base
	diff.Name = "renamed"
	if !profileIdentityEqual(base, diff) {
		t.Error("name change should not invalidate connection identity")
	}
}

func TestConnectionIdentityChanged(t *testing.T) {
	base := store.Profile{Name: "a", Host: "h", Port: 22, User: "u", Auth: store.AuthPassword, Password: "p"}
	if connectionIdentityChanged(base, base) {
		t.Error("no change should not trigger rebuild")
	}
	diff := base
	diff.Host = "other"
	if !connectionIdentityChanged(base, diff) {
		t.Error("host change should trigger rebuild")
	}
	diff = base
	diff.Tags = []string{"new"}
	if connectionIdentityChanged(base, diff) {
		t.Error("tag change should NOT trigger rebuild")
	}
	diff = base
	diff.Options = map[string]string{"ConnectTimeout": "5s"}
	if connectionIdentityChanged(base, diff) {
		t.Error("option change should NOT trigger rebuild")
	}
}
