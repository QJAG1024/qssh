package privacy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultEnabled(t *testing.T) {
	// Isolate sticky/env.
	t.Setenv("QSSH_PRIVACY", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	mu.Lock()
	revealOnce = false
	stickyLoaded = false
	stickyValue = nil
	mu.Unlock()

	if !Enabled() {
		t.Fatal("default should be privacy ON")
	}
	h := Host("192.168.1.1")
	if h != Redacted {
		t.Fatalf("Host: got %q want %q", h, Redacted)
	}
}

func TestRevealOnce(t *testing.T) {
	t.Setenv("QSSH_PRIVACY", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	mu.Lock()
	revealOnce = false
	stickyLoaded = false
	stickyValue = nil
	mu.Unlock()

	RevealOnce()
	if Enabled() {
		t.Fatal("reveal should disable privacy")
	}
	if Host("10.0.0.1") != "10.0.0.1" {
		t.Fatal("Host should pass through when revealed")
	}
}

func TestSticky(t *testing.T) {
	rt := t.TempDir()
	t.Setenv("QSSH_PRIVACY", "")
	t.Setenv("XDG_RUNTIME_DIR", rt)
	mu.Lock()
	revealOnce = false
	stickyLoaded = false
	stickyValue = nil
	mu.Unlock()

	if err := SetSticky("off"); err != nil {
		t.Fatal(err)
	}
	if Enabled() {
		t.Fatal("sticky off should disable")
	}
	// File should exist under runtime/qssh/privacy
	if _, err := os.Stat(filepath.Join(rt, "qssh", "privacy")); err != nil {
		t.Fatalf("sticky file: %v", err)
	}

	if err := SetSticky("on"); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("sticky on should enable")
	}

	if err := SetSticky("clear"); err != nil {
		t.Fatal(err)
	}
	// default back to on
	if !Enabled() {
		t.Fatal("cleared sticky should return to default ON")
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("QSSH_PRIVACY", "off")
	mu.Lock()
	revealOnce = false
	stickyLoaded = false
	stickyValue = nil
	mu.Unlock()

	if Enabled() {
		t.Fatal("env off should disable")
	}
	en, src := Status()
	if en || src != "env" {
		t.Fatalf("status: enabled=%v source=%s", en, src)
	}
}

func TestAddrAndUserAt(t *testing.T) {
	t.Setenv("QSSH_PRIVACY", "on")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	mu.Lock()
	revealOnce = false
	stickyLoaded = false
	stickyValue = nil
	mu.Unlock()

	if got := Addr("192.168.1.1:22"); got != "***:22" {
		t.Fatalf("Addr: %q", got)
	}
	if got := UserAt("root", "192.168.1.1", 22); got != "root@***" {
		t.Fatalf("UserAt: %q", got)
	}
}

func TestScrubIPs(t *testing.T) {
	t.Setenv("QSSH_PRIVACY", "on")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	mu.Lock()
	revealOnce = false
	stickyLoaded = false
	stickyValue = nil
	mu.Unlock()

	in := "ssh dial: dial tcp 192.168.10.177:22: i/o timeout"
	out := Scrub(in)
	if strings.Contains(out, "192.168.10.177") {
		t.Fatalf("still has IP: %q", out)
	}
	if !strings.Contains(out, "***:22") {
		t.Fatalf("expected redacted port form: %q", out)
	}

	in2 := "dns resolve: lookup example.com: no such host"
	out2 := Scrub(in2)
	if strings.Contains(out2, "example.com") {
		t.Fatalf("hostname in lookup not scrubbed: %q", out2)
	}
}

func TestScrubOff(t *testing.T) {
	t.Setenv("QSSH_PRIVACY", "off")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	mu.Lock()
	revealOnce = false
	stickyLoaded = false
	stickyValue = nil
	mu.Unlock()

	in := "dial tcp 1.2.3.4:22"
	if Scrub(in) != in {
		t.Fatalf("should pass through when off")
	}
}
