package keyring

import (
	"os"
	"path/filepath"
	"testing"
)

// isolatePATH removes secret-tool from PATH so tests don't hit the real keyring.
func isolatePATH(t *testing.T) {
	t.Helper()
	// Empty dir first so LookPath finds nothing.
	t.Setenv("PATH", t.TempDir())
}

func TestKeyring_FileBased(t *testing.T) {
	isolatePATH(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")

	kr := New(keyPath, BackendFile)

	// First call: should generate and store a new key (no store yet).
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

	// Second call on same instance: cached.
	key1b, err := kr.Get()
	if err != nil {
		t.Fatalf("cached Get: %v", err)
	}
	for i := range key1 {
		if key1[i] != key1b[i] {
			t.Fatal("cached key differs")
		}
	}

	// New instance: should read the same key back.
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

func TestKeyring_RefuseGenerateWhenStoreExists(t *testing.T) {
	isolatePATH(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")

	kr := New(keyPath, BackendFile)
	kr.SetStoreExists(true)

	_, err := kr.Get()
	if err == nil {
		t.Fatal("expected error refusing to generate when store exists")
	}
	if _, statErr := os.Stat(keyPath); !os.IsNotExist(statErr) {
		t.Fatal("must not write store.key when refusing generate")
	}
}

func TestKeyring_GenerateUniqueKeys(t *testing.T) {
	isolatePATH(t)
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
	isolatePATH(t)
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

func TestKeyring_CacheStable(t *testing.T) {
	isolatePATH(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")
	kr := New(keyPath, BackendFile)
	k1, err := kr.Get()
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the on-disk key; cache must still return original.
	os.WriteFile(keyPath, []byte("00"), 0600)
	k2, err := kr.Get()
	if err != nil {
		t.Fatal(err)
	}
	for i := range k1 {
		if k1[i] != k2[i] {
			t.Fatal("Get after disk corruption should still return cached key")
		}
	}
}
