package internal

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOptionKeys(t *testing.T) {
	// Valid keys pass.
	if errs := ValidateOptionKeys([]string{"ConnectTimeout", "SetEnv", "term.mode", "hostkey.mode", "history.record_commands", "sftp.bind"}); len(errs) != 0 {
		t.Errorf("valid keys rejected: %v", errs)
	}
	// Unknown key rejected.
	errs := ValidateOptionKeys([]string{"term.mode", "bogus.key"})
	if len(errs) != 1 || !strings.Contains(errs[0], "bogus.key") {
		t.Errorf("unknown key: got %v, want 1 error mentioning bogus.key", errs)
	}
	// Global-only key gets the dedicated hint.
	errs = ValidateOptionKeys([]string{"sftp.allow_non_loopback"})
	if len(errs) != 1 || !strings.Contains(errs[0], "global-only") {
		t.Errorf("global-only key: got %v, want global-only hint", errs)
	}
}

func TestMergeSetEnv(t *testing.T) {
	// Add one variable keeps the others (sorted output).
	got := mergeSetEnv("LANG=en_US.UTF-8,LC_ALL=C", "FOO=bar")
	want := "FOO=bar,LANG=en_US.UTF-8,LC_ALL=C"
	if got != want {
		t.Errorf("merge add: got %q, want %q", got, want)
	}
	// Update existing variable.
	got = mergeSetEnv("A=1,B=2", "B=3")
	if got != "A=1,B=3" {
		t.Errorf("merge update: got %q, want A=1,B=3", got)
	}
	// Empty value removes.
	got = mergeSetEnv("A=1,B=2", "A=")
	if got != "B=2" {
		t.Errorf("merge remove: got %q, want B=2", got)
	}
	// Removing the last variable yields empty.
	got = mergeSetEnv("A=1", "A=")
	if got != "" {
		t.Errorf("merge remove last: got %q, want empty", got)
	}
	// Empty existing + add works.
	got = mergeSetEnv("", "X=y")
	if got != "X=y" {
		t.Errorf("merge from empty: got %q", got)
	}
}

func TestApplyOptionMap(t *testing.T) {
	// Valid map applies and merges SetEnv.
	dst, err := ApplyOptionMap(map[string]string{"SetEnv": "A=1"}, map[string]string{"SetEnv": "B=2", "ConnectTimeout": "30s"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if dst["ConnectTimeout"] != "30s" {
		t.Errorf("ConnectTimeout not applied: %v", dst)
	}
	if dst["SetEnv"] != "A=1,B=2" {
		t.Errorf("SetEnv not merged: %q", dst["SetEnv"])
	}

	// Invalid key: destination untouched.
	dst = map[string]string{"ConnectTimeout": "10s"}
	_, err = ApplyOptionMap(dst, map[string]string{"nope": "1"})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if dst["ConnectTimeout"] != "10s" || len(dst) != 1 {
		t.Errorf("destination mutated on error: %v", dst)
	}
}

func TestEffectiveOption(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "qssh", "config.json")

	// No config, no profile opts -> "".
	if got := EffectiveOption(nil, "term.mode"); got != "" {
		t.Errorf("no config: got %q, want empty", got)
	}

	// Global value applies.
	if err := OpenConfig(cfgPath).Set("term.mode", "compat"); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveOption(nil, "term.mode"); got != "compat" {
		t.Errorf("global: got %q, want compat", got)
	}

	// Profile wins over global.
	if got := EffectiveOption(map[string]string{"term.mode": "passthrough"}, "term.mode"); got != "passthrough" {
		t.Errorf("profile override: got %q, want passthrough", got)
	}

	// Empty profile value clears override -> falls to global.
	if got := EffectiveOption(map[string]string{"term.mode": "  "}, "term.mode"); got != "compat" {
		t.Errorf("empty profile value: got %q, want global compat", got)
	}
}
