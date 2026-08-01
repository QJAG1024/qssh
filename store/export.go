package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// Export file format:
//
//	QSSHX1:<base64 json>
//
// where the json is:
//
//	{"salt":"...","nonce":"...","data":"..."}
//
// key = PBKDF2(passphrase, salt, 600000, 32, SHA-256)
// data = AES-256-GCM(key, nonce, profilePayload)
//
// The magic "QSSHX1" prefix lets import detect a non-qssh file immediately,
// and the version digit lets future format changes fail with a clear message
// instead of a confusing parse error.

const (
	exportMagic    = "QSSHX"
	exportVersion  = 1
	pbkdf2Iter     = 600000
	exportKeyBytes = 32
)

// exportEnvelope is the on-disk (base64-encoded) structure.
type exportEnvelope struct {
	Salt  string `json:"salt"`
	Nonce string `json:"nonce"`
	Data  string `json:"data"`
}

// exportPayload is the plaintext inside the envelope: one profile without its
// name (the importer supplies the name). KeyData is the raw private-key file
// content when the key file existed at export time (so cross-machine imports
// can restore it); it is empty otherwise.
type exportPayload struct {
	Host            string            `json:"host"`
	Port            int               `json:"port"`
	User            string            `json:"user"`
	Auth            AuthMethod        `json:"auth"`
	Password        string            `json:"password,omitempty"`
	KeyPath         string            `json:"key_path,omitempty"`
	KeyPassphrase   string            `json:"key_passphrase,omitempty"`
	KeyData         []byte            `json:"key_data,omitempty"` // raw key file, base64 in JSON
	Options         map[string]string `json:"options,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	ProxyDropped    bool              `json:"proxy_dropped,omitempty"` // true if Proxy was non-empty at export
}

// IsExportFile reports whether data looks like a qssh export (magic+version).
func IsExportFile(data []byte) bool {
	s := string(data)
	return strings.HasPrefix(s, exportMagic) && len(s) > len(exportMagic)+1 &&
		s[len(exportMagic)] >= '0' && s[len(exportMagic)] <= '9'
}

// ExportVersion returns the format version of data, or 0 if not a valid export.
func ExportVersion(data []byte) int {
	if !IsExportFile(data) {
		return 0
	}
	s := string(data)
	return int(s[len(exportMagic)] - '0')
}

// ExportProfile encrypts a profile (name stripped) with the passphrase and
// returns the export file bytes (magic prefix + base64 envelope).
// keyData is the raw private-key file content to embed (may be nil).
func ExportProfile(p Profile, passphrase string, keyData []byte) ([]byte, error) {
	payload := exportPayload{
		Host:          p.Host,
		Port:          p.Port,
		User:          p.User,
		Auth:          p.Auth,
		Password:      p.Password,
		KeyPath:       p.KeyPath,
		KeyPassphrase: p.KeyPassphrase,
		KeyData:       keyData,
		Options:       p.Options,
		Tags:          p.Tags,
		ProxyDropped:  p.Proxy != "",
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal profile: %w", err)
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("salt: %w", err)
	}
	key := deriveKey(passphrase, salt)
	nonce, data, err := encryptWithKey(key, plain)
	if err != nil {
		return nil, err
	}
	env, err := json.Marshal(exportEnvelope{Salt: base64.StdEncoding.EncodeToString(salt), Nonce: nonce, Data: data})
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	out := fmt.Sprintf("%s%d:%s", exportMagic, exportVersion, base64.StdEncoding.EncodeToString(env))
	return []byte(out), nil
}

// ImportProfile decrypts an export file and returns the payload.
// Returns a typed error for wrong-passphrase vs corrupt/unsupported input so
// callers can distinguish and report clearly.
func ImportProfile(data []byte, passphrase string) (*exportPayload, error) {
	if !IsExportFile(data) {
		return nil, fmt.Errorf("not a qssh export file (missing %s header)", exportMagic)
	}
	ver := ExportVersion(data)
	if ver != exportVersion {
		return nil, fmt.Errorf("unsupported export version %d (this build supports %d)", ver, exportVersion)
	}
	s := string(data)
	envB64 := s[len(exportMagic)+1+1:] // skip "QSSHX" + digit + ":"
	envRaw, err := base64.StdEncoding.DecodeString(envB64)
	if err != nil {
		return nil, fmt.Errorf("corrupt export: invalid base64: %w", err)
	}
	var env exportEnvelope
	if err := json.Unmarshal(envRaw, &env); err != nil {
		return nil, fmt.Errorf("corrupt export: bad envelope: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, fmt.Errorf("corrupt export: bad salt: %w", err)
	}
	key := deriveKey(passphrase, salt)
	plain, err := decryptWithKey(key, env.Nonce, env.Data)
	if err != nil {
		return nil, ErrWrongPassphraseOrCorrupt
	}
	var payload exportPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("corrupt export: bad payload: %w", err)
	}
	return &payload, nil
}

// ErrWrongPassphraseOrCorrupt is returned when decryption fails — either the
// passphrase is wrong or the file was tampered. Callers present a message that
// covers both.
var ErrWrongPassphraseOrCorrupt = fmt.Errorf("wrong passphrase or corrupted file")

// deriveKey runs PBKDF2-SHA256 with the given salt.
func deriveKey(passphrase string, salt []byte) []byte {
	return pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iter, exportKeyBytes, sha256.New)
}

// encryptWithKey is AES-256-GCM with a random nonce; returns base64 strings.
func encryptWithKey(key, plaintext []byte) (nonceB64, dataB64 string, err error) {
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
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(nonce), base64.StdEncoding.EncodeToString(ct), nil
}

// decryptWithKey decrypts AES-256-GCM base64 nonce+data.
func decryptWithKey(key []byte, nonceB64, dataB64 string) ([]byte, error) {
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("bad nonce size")
	}
	if len(data) < gcm.Overhead() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, nonce, data, nil)
}
