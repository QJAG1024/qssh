package sftpproxy

import (
	"strings"
	"testing"

	"qssh/internal"
	"qssh/internal/daemonstate"
)

// isolateState points the SFTP state file at a temp path so tests never
// touch the developer's real config on any platform (UserConfigDir differs
// macOS/Windows vs Linux).
func isolateState(t *testing.T) {
	t.Setenv("QSSH_SFTP_STATE", t.TempDir()+"/sftp.json")
}

func TestStateSaveLoadRoundtrip(t *testing.T) {
	isolateState(t)
	f := stateFile()
	_ = f.SetEntry("p1", daemonstate.Entry{Port: 1234, PID: 42, Status: daemonstate.StatusReady, URL: "sftp://127.0.0.1:1234"})
	_ = f.SetEntry("p2", daemonstate.Entry{Port: 5678, PID: 43, Status: daemonstate.StatusStarting, Message: "connecting"})

	got := f.All()
	if len(got) != 2 {
		t.Fatalf("All after SetEntry = %d entries, want 2", len(got))
	}
	if got["p1"].Port != 1234 || got["p1"].Status != daemonstate.StatusReady {
		t.Errorf("p1 roundtrip = %+v", got["p1"])
	}
	if got["p2"].Message != "connecting" {
		t.Errorf("p2 roundtrip = %+v", got["p2"])
	}
}

func TestLoadStateMissingFile(t *testing.T) {
	isolateState(t)
	got := stateFile().All()
	if len(got) != 0 {
		t.Errorf("missing file should yield empty map, got %d", len(got))
	}
}

func TestStopNotRunning(t *testing.T) {
	isolateState(t)
	err := Stop("nonexistent")
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("Stop on missing profile = %v, want 'not running'", err)
	}
}

func TestStopBarePIDRefused(t *testing.T) {
	isolateState(t)
	// Pre-identity state: PID set but no starttime/exe.
	_ = stateFile().SetEntry("p", daemonstate.Entry{Port: 1234, PID: 99, Status: daemonstate.StatusReady})
	err := Stop("p")
	if err == nil || !strings.Contains(err.Error(), "lacks start-time/exe identity") {
		t.Errorf("Stop bare PID = %v, want refusal", err)
	}
	// State should be cleaned up even though we refused to signal.
	if _, ok := stateFile().Get("p"); ok {
		t.Error("bare-PID state should be cleaned after refusal")
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("process already finished", "not reachable", "process already finished") {
		t.Error("should match")
	}
	if containsAny("permission denied", "not reachable") {
		t.Error("should not match")
	}
}

func TestMarkReadyAndRemoveState(t *testing.T) {
	isolateState(t)
	markReady("p", 9000, "sftp://127.0.0.1:9000", internal.CurrentIdentity(), "fp123")
	e, ok := stateFile().Get("p")
	if !ok || e.Status != daemonstate.StatusReady || e.Port != 9000 || e.Fingerprint != "fp123" {
		t.Errorf("markReady = %+v", e)
	}
	RemoveState("p")
	if _, ok := stateFile().Get("p"); ok {
		t.Error("RemoveState should delete entry")
	}
}
