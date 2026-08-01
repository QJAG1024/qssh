package store

import (
	"testing"
)

// BenchmarkStoreEncrypt estimates AES-256-GCM encrypt + base64 of a small store.
func BenchmarkStoreEncrypt(b *testing.B) {
	key := make([]byte, 32)
	payload := make([]byte, 4096) // ~a handful of profiles
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nonce, data, err := encrypt(key, payload)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = nonce, data
	}
}

// BenchmarkStoreDecrypt estimates AES-256-GCM decrypt of the same payload.
func BenchmarkStoreDecrypt(b *testing.B) {
	key := make([]byte, 32)
	payload := make([]byte, 4096)
	nonce, data, _ := encrypt(key, payload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := decrypt(key, nonce, data); err != nil {
			b.Fatal(err)
		}
	}
}
