package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qssh/keyring"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")
	storePath := filepath.Join(dir, "store.json")
	kr := keyring.New(keyPath, keyring.BackendFile)
	s, err := New(storePath, kr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, storePath
}

func TestStore_AddAndGet(t *testing.T) {
	s, _ := newTestStore(t)

	p := Profile{
		Name:     "test",
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		Auth:     AuthPassword,
		Password: "secret",
	}
	if err := s.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := s.Get("test")
	if !ok {
		t.Fatal("Get returned not found")
	}
	if got.Host != "192.168.1.1" || got.User != "root" {
		t.Fatalf("got %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set")
	}
}

func TestStore_AddDuplicate(t *testing.T) {
	s, _ := newTestStore(t)
	p := Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}
	s.Add(p)
	err := s.Add(p)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got: %v", err)
	}
}

func TestStore_Update(t *testing.T) {
	s, _ := newTestStore(t)
	s.Add(Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})

	p2 := Profile{Name: "x", Host: "newhost", Port: 2222, User: "u2", Auth: AuthPassword, Password: "p2"}
	if err := s.Update("x", p2); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := s.Get("x")
	if got.Host != "newhost" || got.Port != 2222 {
		t.Fatalf("got %+v", got)
	}
}

func TestStore_UpdateNotFound(t *testing.T) {
	s, _ := newTestStore(t)
	err := s.Update("nonexistent", Profile{
		Name: "nonexistent", Host: "h", Port: 22, User: "u",
		Auth: AuthPassword, Password: "p",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got: %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	s, _ := newTestStore(t)
	s.Add(Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
	if err := s.Delete("x"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("x"); ok {
		t.Fatal("profile should be deleted")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	s, _ := newTestStore(t)
	err := s.Delete("nonexistent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got: %v", err)
	}
}

func TestStore_List(t *testing.T) {
	s, _ := newTestStore(t)
	names := []string{"z", "a", "m"}
	for _, n := range names {
		s.Add(Profile{Name: n, Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
	}
	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	// Should be sorted.
	if list[0] != "a" || list[1] != "m" || list[2] != "z" {
		t.Fatalf("expected sorted order, got %v", list)
	}
}

func TestStore_Search(t *testing.T) {
	s, _ := newTestStore(t)
	s.Add(Profile{Name: "web-prod", Host: "10.0.0.1", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
	s.Add(Profile{Name: "db-prod", Host: "10.0.0.2", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
	s.Add(Profile{Name: "dev-box", Host: "192.168.1.10", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})

	r := s.Search("prod")
	if len(r) != 2 {
		t.Fatalf("expected 2 results for 'prod', got %d", len(r))
	}

	r = s.Search("10.0")
	if len(r) != 2 {
		t.Fatalf("expected 2 results for '10.0', got %d", len(r))
	}

	r = s.Search("nonexistent")
	if len(r) != 0 {
		t.Fatalf("expected 0, got %d", len(r))
	}
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")
	storePath := filepath.Join(dir, "store.json")

	kr := keyring.New(keyPath, keyring.BackendFile)
	s1, err := New(storePath, kr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s1.Add(Profile{Name: "persist-test", Host: "10.0.0.1", Port: 22, User: "root", Auth: AuthPassword, Password: "secret"})

	// Open a new store instance pointing at the same file.
	kr2 := keyring.New(keyPath, keyring.BackendFile)
	s2, err := New(storePath, kr2)
	if err != nil {
		t.Fatalf("New second instance: %v", err)
	}
	got, ok := s2.Get("persist-test")
	if !ok {
		t.Fatal("profile not found after reload")
	}
	if got.Host != "10.0.0.1" || got.User != "root" {
		t.Fatalf("got %+v", got)
	}
}

func TestStore_Encryption(t *testing.T) {
	s, storePath := newTestStore(t)
	// Add a profile so the store is saved to disk.
	s.Add(Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})

	// Read the raw file — it should be encrypted JSON, not plaintext.
	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read store file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "profiles") {
		t.Fatal("store file contains plaintext 'profiles' — data is not encrypted")
	}
	if !strings.Contains(content, `"encrypted": true`) {
		t.Fatal("store file missing encrypted flag")
	}
}

func TestStore_Touch(t *testing.T) {
	s, _ := newTestStore(t)
	s.Add(Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})

	s.Touch("x")
	p, _ := s.Get("x")
	if p.ConnectionCount != 1 {
		t.Fatalf("expected connection count 1, got %d", p.ConnectionCount)
	}
	if p.LastUsed.IsZero() {
		t.Fatal("LastUsed should be set after Touch")
	}

	s.Touch("x")
	p, _ = s.Get("x")
	if p.ConnectionCount != 2 {
		t.Fatalf("expected connection count 2, got %d", p.ConnectionCount)
	}
}

func TestStore_GetAll(t *testing.T) {
	s, _ := newTestStore(t)
	s.Add(Profile{Name: "a", Host: "h1", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
	s.Add(Profile{Name: "b", Host: "h2", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})

	all := s.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestStore_CorruptedFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")
	storePath := filepath.Join(dir, "store.json")

	kr := keyring.New(keyPath, keyring.BackendFile)
	s, _ := New(storePath, kr)
	s.Add(Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})

	// Corrupt the file.
	os.WriteFile(storePath, []byte("garbage"), 0600)

	kr2 := keyring.New(keyPath, keyring.BackendFile)
	_, err := New(storePath, kr2)
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
}