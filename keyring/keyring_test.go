package keyring

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyring_FileBased(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")

	kr := New(keyPath, BackendFile)

	// First call: should generate and store a new key.
	key1, err := kr.Get()
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(key1))
	}

	// Verify the key was written to disk.
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("key file was not created")
	}

	// Second call: should read the same key back.
	kr2 := New(keyPath, BackendFile)
	key2, err := kr2.Get()
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if len(key2) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(key2))
	}
	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("keys don't match between Get calls")
		}
	}
}

func TestKeyring_GenerateUniqueKeys(t *testing.T) {
	dir := t.TempDir()

	kr1 := New(filepath.Join(dir, "k1"), BackendFile)
	kr2 := New(filepath.Join(dir, "k2"), BackendFile)

	k1, _ := kr1.Get()
	k2, _ := kr2.Get()

	// Extremely unlikely to collide.
	equal := true
	for i := range k1 {
		if k1[i] != k2[i] {
			equal = false
			break
		}
	}
	if equal {
		t.Fatal("two generated keys should not be identical")
	}
}

func TestKeyring_InvalidHexFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")

	// Write invalid hex to the key file.
	os.WriteFile(keyPath, []byte("not-hex\n"), 0600)

	kr := New(keyPath, BackendFile)
	_, err := kr.Get()
	if err == nil {
		t.Fatal("expected error for invalid hex, got nil")
	}
}