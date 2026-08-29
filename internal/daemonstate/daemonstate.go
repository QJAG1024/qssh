// Package daemonstate implements the per-service daemon state files
// (sftp.json, webdav.json): a map of profile name -> Entry, persisted as
// JSON with cross-process-safe read-modify-write.
//
// It deliberately extracts only the shared mechanics: entry shape, file
// locking, atomic writes, and a process-liveness lookup. It does NOT encode
// lifecycle semantics (e.g. whether a failed entry is kept or removed,
// whether Stop kills processes) — those differ between the SFTP proxy and
// the WebDAV daemon and stay in their respective packages.
package daemonstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"qssh/internal"
)

// Entry status values used by the services.
const (
	StatusStarting = "starting"
	StatusReady    = "ready"
	StatusFailed   = "failed"
)

// Entry is one daemon's state record. Fields that only one service uses
// (Token for WebDAV, Fingerprint for SFTP) live here with omitempty so the
// on-disk format of each service's file stays byte-compatible.
type Entry struct {
	Port        int    `json:"port"`
	PID         int    `json:"pid"`
	StartTime   uint64 `json:"start_time,omitempty"`
	Exe         string `json:"exe,omitempty"`
	URL         string `json:"url"`
	Status      string `json:"status"` // starting | ready | failed
	Message     string `json:"message,omitempty"`
	Token       string `json:"token,omitempty"`       // WebDAV only
	Fingerprint string `json:"fingerprint,omitempty"` // SFTP only
}

// Identity returns the entry's process identity for use with
// internal.MatchIdentity / GracefulStopIdent.
func (e Entry) Identity() internal.ProcessIdentity {
	return internal.ProcessIdentity{PID: e.PID, StartTime: e.StartTime, Exe: e.Exe}
}

// HasIdentity reports whether the entry carries start-time or exe identity.
// Entries without identity (pre-identity state files) must never be used to
// signal a bare PID.
func (e Entry) HasIdentity() bool {
	return e.StartTime != 0 || e.Exe != ""
}

// File is a handle to one state file. All mutations take an in-process
// mutex plus a cross-process flock on path+".lock", then reload the file
// before applying changes so concurrent qssh invocations cannot lose
// updates to each other's entries.
type File struct {
	path string
	mu   sync.Mutex
}

// Open returns a handle for the state file at path. The file may not exist
// yet; it is created on first write.
func Open(path string) *File {
	return &File{path: path}
}

// Path returns the state file path.
func (f *File) Path() string { return f.path }

// Get returns the entry for name (a fresh load from disk).
func (f *File) Get(name string) (Entry, bool) {
	all := f.All()
	e, ok := all[name]
	return e, ok
}

// All returns every entry (a fresh load from disk). A missing or corrupt
// file yields an empty map — state files are advisory, never fatal.
func (f *File) All() map[string]Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loadLocked()
}

func (f *File) loadLocked() map[string]Entry {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return map[string]Entry{}
	}
	var m map[string]Entry
	json.Unmarshal(data, &m)
	if m == nil {
		m = make(map[string]Entry)
	}
	return m
}

// With runs fn on the freshly-loaded entry map while holding the
// cross-process lock, then persists the map if fn returns nil. A corrupt or
// missing file starts fn from an empty map. Callers that do not care about
// write failures may ignore the return value (the services historically
// treat state writes as best-effort).
func (f *File) With(fn func(map[string]Entry) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	fl, err := internal.Lock(f.path)
	if err != nil {
		return fmt.Errorf("lock daemon state: %w", err)
	}
	defer fl.Unlock()

	m := f.loadLocked()
	if err := fn(m); err != nil {
		return err
	}
	return f.saveLocked(m)
}

// SetEntry replaces (or creates) the entry for name.
func (f *File) SetEntry(name string, e Entry) error {
	return f.With(func(m map[string]Entry) error {
		m[name] = e
		return nil
	})
}

// SetMessage updates only Message for an existing entry (no-op when the
// entry does not exist — progress updates must not resurrect entries).
func (f *File) SetMessage(name, msg string) error {
	return f.With(func(m map[string]Entry) error {
		if e, ok := m[name]; ok {
			e.Message = msg
			m[name] = e
		}
		return nil
	})
}

// DeleteEntry removes the entry for name (no-op when absent).
func (f *File) DeleteEntry(name string) error {
	return f.With(func(m map[string]Entry) error {
		delete(m, name)
		return nil
	})
}

func (f *File) saveLocked(m map[string]Entry) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	// Atomic write: temp file in the same directory + rename, so a crash
	// cannot leave a truncated state file.
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(f.path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := internal.ReplaceFile(tmpName, f.path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
