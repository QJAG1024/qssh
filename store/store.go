package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"qssh/internal"
	"qssh/keyring"
)

// FormatVersion is the current store file format version.
const FormatVersion = 1

// Store manages encrypted SSH credential profiles on disk.
type Store struct {
	mu       sync.RWMutex
	path     string
	keyring  *keyring.Keyring
	profiles map[string]Profile
	dirty    bool
}

// encryptedFile is the JSON structure written to disk.
type encryptedFile struct {
	Encrypted bool   `json:"encrypted"`
	Nonce     string `json:"nonce"`
	Data      string `json:"data"`
	Version   int    `json:"version"`
}

// plainData is the JSON plaintext before encryption.
type plainData struct {
	Profiles map[string]Profile `json:"profiles"`
	Version  int                `json:"version"`
}

// New opens or initializes the credential store.
// If the store file doesn't exist, it creates an empty encrypted store.
// When the store file exists, the keyring is told not to mint a new master key
// (avoids silent re-keying when the login keyring is locked after reboot).
func New(storePath string, kr *keyring.Keyring) (*Store, error) {
	s := &Store{
		path:     storePath,
		keyring:  kr,
		profiles: make(map[string]Profile),
	}
	// Guard the whole initialization so concurrent first-time creation and
	// loading do not race on the directory or the store file.
	s.mu.Lock()
	defer s.mu.Unlock()
	return s, s.withFileLock(func() error {
		if _, err := os.Stat(storePath); os.IsNotExist(err) {
			// Brand-new store — allow key minting, ensure directory exists.
			kr.SetStoreExists(false)
			dir := filepath.Dir(storePath)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("create store dir: %w", err)
			}
			s.dirty = true
			return nil
		}
		// Existing encrypted store — never generate a replacement key.
		kr.SetStoreExists(true)
		return s.load()
	})
}

// Add inserts a new profile. Returns error if name already exists.
// Holds a cross-process lock and reloads before write so concurrent
// `qssh --add` invocations cannot lose updates.
func (s *Store) Add(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
		if err := s.reloadLocked(); err != nil {
			return err
		}
		if _, exists := s.profiles[p.Name]; exists {
			return fmt.Errorf("profile %q already exists (use --edit to modify)", p.Name)
		}
		now := time.Now()
		p.CreatedAt = now
		p.UpdatedAt = now
		s.profiles[p.Name] = p
		s.dirty = true
		return s.save()
	})
}

// Get retrieves a profile by name.
func (s *Store) Get(name string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[name]
	return p, ok
}

// Update replaces an existing profile. Returns error if not found.
func (s *Store) Update(name string, p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
		if err := s.reloadLocked(); err != nil {
			return err
		}
		existing, exists := s.profiles[name]
		if !exists {
			return fmt.Errorf("profile %q not found", name)
		}
		p.UpdatedAt = time.Now()
		// Preserve CreatedAt, LastUsed, ConnectionCount.
		p.CreatedAt = existing.CreatedAt
		if p.LastUsed.IsZero() {
			p.LastUsed = existing.LastUsed
		}
		if p.ConnectionCount == 0 {
			p.ConnectionCount = existing.ConnectionCount
		}
		s.profiles[name] = p
		s.dirty = true
		return s.save()
	})
}

// Delete removes a profile. Returns error if not found.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
		if err := s.reloadLocked(); err != nil {
			return err
		}
		if _, exists := s.profiles[name]; !exists {
			return fmt.Errorf("profile %q not found", name)
		}
		delete(s.profiles, name)
		s.dirty = true
		return s.save()
	})
}

// Rename renames a profile in a single atomic save. Returns error if old
// is missing or new already exists.
func (s *Store) Rename(oldName, newName string) error {
	if newName == "" {
		return fmt.Errorf("new profile name is required")
	}
	if oldName == newName {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withFileLock(func() error {
		if err := s.reloadLocked(); err != nil {
			return err
		}
		p, exists := s.profiles[oldName]
		if !exists {
			return fmt.Errorf("profile %q not found", oldName)
		}
		if _, exists := s.profiles[newName]; exists {
			return fmt.Errorf("profile %q already exists", newName)
		}
		// Validate with the new name before mutating.
		p.Name = newName
		if err := p.Validate(); err != nil {
			return err
		}
		p.UpdatedAt = time.Now()
		delete(s.profiles, oldName)
		s.profiles[newName] = p
		s.dirty = true
		return s.save()
	})
}

// List returns all profile names sorted alphabetically.
func (s *Store) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.profiles))
	for n := range s.profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// GetAll returns all profiles as a name->profile map.
func (s *Store) GetAll() map[string]Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := make(map[string]Profile, len(s.profiles))
	for k, v := range s.profiles {
		m[k] = v
	}
	return m
}

// Search returns profiles whose name or host matches the query (case-insensitive).
func (s *Store) Search(query string) []Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	var results []Profile
	for _, p := range s.profiles {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Host), q) {
			results = append(results, p)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results
}

// Touch updates LastUsed and ConnectionCount after a successful connection.
func (s *Store) Touch(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Best-effort — don't fail the connection for a lock/save error.
	_ = s.withFileLock(func() error {
		if err := s.reloadLocked(); err != nil {
			return err
		}
		p, ok := s.profiles[name]
		if !ok {
			return nil
		}
		p.LastUsed = time.Now()
		p.ConnectionCount++
		s.profiles[name] = p
		s.dirty = true
		return s.save()
	})
}

// --- encryption / persistence ---

// withFileLock runs fn while holding the store's cross-process flock.
// Caller must already hold s.mu.
func (s *Store) withFileLock(fn func() error) error {
	fl, err := internal.Lock(s.path)
	if err != nil {
		return fmt.Errorf("lock store: %w", err)
	}
	defer fl.Unlock()
	return fn()
}

// reloadLocked re-reads the encrypted store from disk into s.profiles.
// Caller must hold s.mu and the file lock. Missing file is treated as empty.
func (s *Store) reloadLocked() error {
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		// Brand-new store never written — keep current (likely empty) map.
		if s.profiles == nil {
			s.profiles = make(map[string]Profile)
		}
		return nil
	}
	return s.load()
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var ef encryptedFile
	if err := json.Unmarshal(data, &ef); err != nil {
		return fmt.Errorf("parse store file: %w", err)
	}
	if !ef.Encrypted {
		return fmt.Errorf("store file is not encrypted")
	}

	key, err := s.keyring.Get()
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	plaintext, err := decrypt(key, ef.Nonce, ef.Data)
	if err != nil {
		return fmt.Errorf("decrypt store: %w", err)
	}

	var pd plainData
	if err := json.Unmarshal(plaintext, &pd); err != nil {
		return fmt.Errorf("parse decrypted data: %w", err)
	}
	s.profiles = pd.Profiles
	if s.profiles == nil {
		s.profiles = make(map[string]Profile)
	}
	for name := range s.profiles {
		if err := validateProfileName(name); err != nil {
			return fmt.Errorf("refuse to load profile with invalid name %q: %w", name, err)
		}
	}
	return nil
}

func (s *Store) save() error {
	if !s.dirty {
		return nil
	}

	pd := plainData{
		Profiles: s.profiles,
		Version:  FormatVersion,
	}
	plaintext, err := json.Marshal(pd)
	if err != nil {
		return fmt.Errorf("marshal profiles: %w", err)
	}

	key, err := s.keyring.Get()
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	nonce, data, err := encrypt(key, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt store: %w", err)
	}

	ef := encryptedFile{
		Encrypted: true,
		Nonce:     nonce,
		Data:      data,
		Version:   FormatVersion,
	}
	out, err := json.MarshalIndent(ef, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal encrypted file: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	// Atomic replace: write temp in the same directory then rename.
	// rename(2) is atomic on the same filesystem, so a crash mid-write
	// cannot leave a truncated store.json.
	tmp, err := os.CreateTemp(dir, ".store-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp store: %w", err)
	}
	tmpName := tmp.Name()
	// Ensure the temp file is removed on any failure path.
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp store: %w", err)
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp store: %w", err)
	}
	if err := internal.ReplaceFile(tmpName, s.path); err != nil {
		return fmt.Errorf("replace store: %w", err)
	}
	cleanup = false
	s.dirty = false
	return nil
}

// encrypt encrypts plaintext with AES-256-GCM.
// Returns base64-encoded nonce and ciphertext.
func encrypt(key, plaintext []byte) (nonceB64, dataB64 string, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts AES-256-GCM ciphertext using the given key.
func decrypt(key []byte, nonceB64, dataB64 string) ([]byte, error) {
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// Guard before Open — wrong nonce size panics inside crypto/cipher.
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("decrypt failed: invalid nonce size %d (want %d)", len(nonce), gcm.NonceSize())
	}
	if len(ciphertext) < gcm.Overhead() {
		return nil, fmt.Errorf("decrypt failed: ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed (wrong key or corrupted data): %w", err)
	}
	return plaintext, nil
}
