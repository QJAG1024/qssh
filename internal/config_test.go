package internal

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConfig_SetGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := OpenConfig(path)
	if err := c.Set("hostkey.mode", "strict"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	c2 := OpenConfig(path)
	if got := c2.Get("hostkey.mode"); got != "strict" {
		t.Fatalf("Get = %q, want strict", got)
	}
}

func TestConfig_ConcurrentSetNoLostUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Seed empty config.
	c := OpenConfig(path)
	if err := c.Set("seed", "1"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			c := OpenConfig(path)
			key := "k" + string(rune('a'+i%26)) + string(rune('0'+i/26))
			// Use unique keys so we can verify all writes survived.
			key = "key-" + itoa(i)
			if err := c.Set(key, "v"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent set: %v", err)
	}

	final := OpenConfig(path)
	all := final.All()
	// seed + n unique keys
	if len(all) < n+1 {
		t.Fatalf("lost updates: got %d keys, want >= %d; all=%v", len(all), n+1, all)
	}
	for i := 0; i < n; i++ {
		k := "key-" + itoa(i)
		if all[k] != "v" {
			t.Errorf("missing key %s", k)
		}
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

func TestConfig_CorruptFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	c := OpenConfig(path)
	if c.LoadError() == nil {
		t.Fatal("expected LoadError for corrupt config")
	}
	// Set must refuse to clobber a corrupt file
	if err := c.Set("hostkey.mode", "strict"); err == nil {
		t.Fatal("Set should refuse corrupt config")
	}
}
