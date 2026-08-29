package daemonstate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	f := Open(filepath.Join(t.TempDir(), "s.json"))
	if err := f.SetEntry("a", Entry{Port: 1, PID: 2, URL: "u", Status: StatusReady, Token: "tok", Fingerprint: "fp"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetEntry("b", Entry{Port: 3, Status: StatusStarting, Message: "m"}); err != nil {
		t.Fatal(err)
	}
	all := f.All()
	if len(all) != 2 {
		t.Fatalf("All = %d entries, want 2", len(all))
	}
	if all["a"].Token != "tok" || all["a"].Fingerprint != "fp" {
		t.Errorf("service-specific fields lost: %+v", all["a"])
	}
	if all["b"].Message != "m" {
		t.Errorf("message roundtrip = %+v", all["b"])
	}

	// A fresh handle sees the same file (state crosses handles/processes).
	if got, ok := Open(f.Path()).Get("a"); !ok || got.Port != 1 {
		t.Errorf("cross-handle Get = %+v, %v", got, ok)
	}
}

func TestMissingFileYieldsEmpty(t *testing.T) {
	f := Open(filepath.Join(t.TempDir(), "nope.json"))
	if len(f.All()) != 0 {
		t.Error("missing file should yield empty map")
	}
	if _, ok := f.Get("x"); ok {
		t.Error("Get on missing file should report absent")
	}
}

func TestSetMessageOnlyUpdatesExisting(t *testing.T) {
	f := Open(filepath.Join(t.TempDir(), "s.json"))
	// No entry: SetMessage must not resurrect/create it.
	if err := f.SetMessage("ghost", "hi"); err != nil {
		t.Fatal(err)
	}
	if len(f.All()) != 0 {
		t.Error("SetMessage must not create entries")
	}
	// Existing entry: message updated.
	_ = f.SetEntry("a", Entry{Port: 1, Status: StatusStarting})
	if err := f.SetMessage("a", "connecting"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Get("a"); got.Message != "connecting" || got.Port != 1 {
		t.Errorf("SetMessage = %+v", got)
	}
}

func TestDeleteEntry(t *testing.T) {
	f := Open(filepath.Join(t.TempDir(), "s.json"))
	_ = f.SetEntry("a", Entry{Status: StatusReady})
	if err := f.DeleteEntry("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Get("a"); ok {
		t.Error("DeleteEntry should remove the entry")
	}
	// Deleting a missing entry is a no-op, not an error.
	if err := f.DeleteEntry("a"); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityHelpers(t *testing.T) {
	e := Entry{PID: 42}
	if e.HasIdentity() {
		t.Error("bare PID must not report identity")
	}
	e.StartTime = 7
	if !e.HasIdentity() {
		t.Error("starttime should count as identity")
	}
	id := e.Identity()
	if id.PID != 42 || id.StartTime != 7 {
		t.Errorf("Identity = %+v", id)
	}
}

func TestWithReloadsBeforeMutating(t *testing.T) {
	// Simulates a concurrent writer: an external process adds an entry
	// between two With transactions; the second transaction must see it.
	path := filepath.Join(t.TempDir(), "s.json")
	f1 := Open(path)
	f2 := Open(path)

	_ = f1.SetEntry("one", Entry{Port: 1, Status: StatusReady})
	// f2 "process" adds its own entry.
	_ = f2.SetEntry("two", Entry{Port: 2, Status: StatusReady})
	// f1's next transaction must preserve f2's entry (reload-under-lock).
	if err := f1.With(func(m map[string]Entry) error { return nil }); err != nil {
		t.Fatal(err)
	}
	all := f2.All()
	if _, ok := all["two"]; !ok {
		t.Fatal("With clobbered a concurrent writer's entry")
	}
	if _, ok := all["one"]; !ok {
		t.Fatal("With lost its own prior entry")
	}
}

func TestFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows has no Unix permission bits; Chmod only toggles the
		// read-only attribute. Access control there is the NT ACL's job.
		t.Skip("unix mode check not applicable on windows")
	}
	f := Open(filepath.Join(t.TempDir(), "s.json"))
	_ = f.SetEntry("a", Entry{Status: StatusReady})
	fi, err := os.Stat(f.Path())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("state file mode = %v, want 0600", fi.Mode().Perm())
	}
}
