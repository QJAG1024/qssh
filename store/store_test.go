package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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
func TestStore_Rename(t *testing.T) {
	s, _ := newTestStore(t)
	s.Add(Profile{Name: "old", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})

	if err := s.Rename("old", "new"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, ok := s.Get("old"); ok {
		t.Fatal("old name should be gone")
	}
	got, ok := s.Get("new")
	if !ok {
		t.Fatal("new name should exist")
	}
	if got.Host != "h" || got.Name != "new" {
		t.Fatalf("got %+v", got)
	}
}

func TestStore_RenameConflict(t *testing.T) {
	s, _ := newTestStore(t)
	s.Add(Profile{Name: "a", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
	s.Add(Profile{Name: "b", Host: "h2", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
	err := s.Rename("a", "b")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists, got: %v", err)
	}
	// Original must still be present after failed rename.
	if _, ok := s.Get("a"); !ok {
		t.Fatal("source should remain after conflict")
	}
}

func TestStore_RenameNotFound(t *testing.T) {
	s, _ := newTestStore(t)
	err := s.Rename("missing", "x")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got: %v", err)
	}
}

func TestStore_AtomicSaveNoTempLeft(t *testing.T) {
	s, storePath := newTestStore(t)
	if err := s.Add(Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Final store must exist; no leftover .store-*.tmp files.
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("store missing: %v", err)
	}
	dir := filepath.Dir(storePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".store-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestStore_RefuseNewKeyWhenEncryptedExists(t *testing.T) {
	// Isolate secret-tool so migration cannot "save" the open.
	t.Setenv("PATH", t.TempDir())

	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.json")
	keyPath := filepath.Join(dir, "store.key")

	// Create a real encrypted store with one profile.
	kr1 := keyring.New(keyPath, keyring.BackendFile)
	s1, err := New(storePath, kr1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Add(Profile{Name: "x", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}); err != nil {
		t.Fatal(err)
	}

	// Wipe the key file — simulates lost key / locked keyring with no file backup.
	os.Remove(keyPath)

	// Opening the existing store must fail, not mint a new key.
	kr2 := keyring.New(keyPath, keyring.BackendFile)
	_, err = New(storePath, kr2)
	if err == nil {
		t.Fatal("expected error when key is missing but store exists")
	}
	// Must not have written a new key either.
	if _, e := os.Stat(keyPath); !os.IsNotExist(e) {
		t.Fatal("must not mint store.key when open fails")
	}
}

func TestStore_ConcurrentAddNoLostUpdate(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")
	storePath := filepath.Join(dir, "store.json")
	// Shared keyring path so all processes/goroutines use same key.
	kr := keyring.New(keyPath, keyring.BackendFile)
	s, err := New(storePath, kr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Seed so key is minted once.
	if err := s.Add(Profile{Name: "seed", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			// Each goroutine opens its own Store handle (like separate processes).
			kr := keyring.New(keyPath, keyring.BackendFile)
			s, err := New(storePath, kr)
			if err != nil {
				errs <- err
				return
			}
			name := "p" + string(rune('a'+i%26)) + string(rune('0'+i/26))
			// Prefer deterministic names
			name = "p" + itoa(i)
			err = s.Add(Profile{Name: name, Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent add: %v", err)
	}

	// Re-open and count.
	kr = keyring.New(keyPath, keyring.BackendFile)
	final, err := New(storePath, kr)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	names := final.List()
	// seed + n
	if len(names) != n+1 {
		t.Fatalf("lost updates: got %d profiles %v, want %d", len(names), names, n+1)
	}
}

func TestStore_LoadRejectsInvalidProfileNames(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "store.key")
	storePath := filepath.Join(dir, "store.json")
	// Isolate PATH so this test does not hit a real secret-tool.
	t.Setenv("PATH", t.TempDir())
	kr := keyring.New(keyPath, keyring.BackendFile)
	s, err := New(storePath, kr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Add(Profile{Name: "valid", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}); err != nil {
		t.Fatalf("add valid: %v", err)
	}
	// Bypass validation to simulate a legacy store that contains an
	// illegal profile name (path traversal / hidden file style).
	s.profiles["../../etc"] = Profile{Name: "../../etc", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}
	s.dirty = true
	if err := s.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Re-opening must fail closed rather than expose an unsafe name.
	_, err = New(storePath, kr)
	if err == nil {
		t.Fatal("expected load to reject invalid profile name")
	}
	want := "invalid name"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to contain %q, got: %v", want, err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
