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
func New(storePath string, kr *keyring.Keyring) (*Store, error) {
	s := &Store{
		path:     storePath,
		keyring:  kr,
		profiles: make(map[string]Profile),
	}
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		// New store — ensure directory exists, save empty, done.
		dir := filepath.Dir(storePath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create store dir: %w", err)
		}
		s.dirty = true
		return s, nil
	}
	// Existing store — load and decrypt.
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load store: %w", err)
	}
	return s, nil
}

// Add inserts a new profile. Returns error if name already exists.
func (s *Store) Add(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.profiles[p.Name]; exists {
		return fmt.Errorf("profile %q already exists (use --edit to modify)", p.Name)
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	s.profiles[p.Name] = p
	s.dirty = true
	return s.save()
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
	if _, exists := s.profiles[name]; !exists {
		return fmt.Errorf("profile %q not found", name)
	}
	p.UpdatedAt = time.Now()
	// Preserve CreatedAt, LastUsed, ConnectionCount.
	if existing, ok := s.profiles[name]; ok {
		p.CreatedAt = existing.CreatedAt
		if p.LastUsed.IsZero() {
			p.LastUsed = existing.LastUsed
		}
		if p.ConnectionCount == 0 {
			p.ConnectionCount = existing.ConnectionCount
		}
	}
	s.profiles[name] = p
	s.dirty = true
	return s.save()
}

// Delete removes a profile. Returns error if not found.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.profiles[name]; !exists {
		return fmt.Errorf("profile %q not found", name)
	}
	delete(s.profiles, name)
	s.dirty = true
	return s.save()
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
	p, ok := s.profiles[name]
	if !ok {
		return
	}
	p.LastUsed = time.Now()
	p.ConnectionCount++
	s.profiles[name] = p
	s.dirty = true
	// Best-effort save — don't fail the connection for a save error.
	_ = s.save()
}

// --- encryption / persistence ---

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
	if err := os.WriteFile(s.path, out, 0600); err != nil {
		return err
	}
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
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed (wrong key or corrupted data): %w", err)
	}
	return plaintext, nil
}