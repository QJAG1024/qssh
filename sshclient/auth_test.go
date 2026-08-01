package sshclient

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qssh/store"
)

func writeTestKey(t *testing.T, encrypted bool) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "id_rsa")
	var block *pem.Block
	if encrypted {
		// Encrypt with "testpass" using PEM encryption.
		block, err = x509.EncryptPEMBlock(rand.Reader, "RSA PRIVATE KEY",
			x509.MarshalPKCS1PrivateKey(key), []byte("testpass"), x509.PEMCipherAES256)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		block = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAuthMethodsForProfile_Password(t *testing.T) {
	methods, err := AuthMethodsForProfile(store.Profile{
		Auth: store.AuthPassword, Password: "hunter2",
	})
	if err != nil {
		t.Fatalf("password auth: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("want 1 method, got %d", len(methods))
	}
}

func TestAuthMethodsForProfile_KeyPlain(t *testing.T) {
	keyPath := writeTestKey(t, false)
	methods, err := AuthMethodsForProfile(store.Profile{
		Auth: store.AuthKey, KeyPath: keyPath,
	})
	if err != nil {
		t.Fatalf("key auth: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("want 1 method, got %d", len(methods))
	}
}

func TestAuthMethodsForProfile_KeyEncrypted(t *testing.T) {
	keyPath := writeTestKey(t, true)
	methods, err := AuthMethodsForProfile(store.Profile{
		Auth: store.AuthKey, KeyPath: keyPath, KeyPassphrase: "testpass",
	})
	if err != nil {
		t.Fatalf("encrypted key auth: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("want 1 method, got %d", len(methods))
	}
}

func TestAuthMethodsForProfile_KeyWrongPassphrase(t *testing.T) {
	keyPath := writeTestKey(t, true)
	_, err := AuthMethodsForProfile(store.Profile{
		Auth: store.AuthKey, KeyPath: keyPath, KeyPassphrase: "wrong",
	})
	if err == nil {
		t.Fatal("wrong passphrase should fail to parse key")
	}
}

func TestAuthMethodsForProfile_KeyMissingFile(t *testing.T) {
	_, err := AuthMethodsForProfile(store.Profile{
		Auth: store.AuthKey, KeyPath: "/nonexistent/key",
	})
	if err == nil || !strings.Contains(err.Error(), "read key file") {
		t.Errorf("missing key = %v, want read error", err)
	}
}

func TestAuthMethodsForProfile_Unsupported(t *testing.T) {
	_, err := AuthMethodsForProfile(store.Profile{Auth: "totally-bogus"})
	if err == nil || !strings.Contains(err.Error(), "unsupported auth method") {
		t.Errorf("unsupported = %v", err)
	}
}

func TestAgentAuth_NoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	_, err := agentAuth()
	if err == nil || !strings.Contains(err.Error(), "SSH_AUTH_SOCK") {
		t.Errorf("no agent sock = %v, want SSH_AUTH_SOCK error", err)
	}
}

func TestAgentAuth_BadSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/nonexistent/agent.sock")
	_, err := agentAuth()
	if err == nil || !strings.Contains(err.Error(), "dial agent") {
		t.Errorf("bad agent sock = %v, want dial error", err)
	}
}
