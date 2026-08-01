package sftpproxy

import (
	"strings"
	"testing"

	"qssh/internal"
)

// isolateState points the SFTP state file at a temp path so tests never
// touch the developer's real config on any platform (UserConfigDir differs
// macOS/Windows vs Linux).
func isolateState(t *testing.T) {
	t.Setenv("QSSH_SFTP_STATE", t.TempDir()+"/sftp.json")
}

func TestStateSaveLoadRoundtrip(t *testing.T) {
	isolateState(t)
	m := map[string]sftpEntry{
		"p1": {Port: 1234, PID: 42, Status: "ready", URL: "sftp://127.0.0.1:1234"},
		"p2": {Port: 5678, PID: 43, Status: "starting", Message: "connecting"},
	}
	saveState(m)

	got := loadState()
	if len(got) != 2 {
		t.Fatalf("loadState after save = %d entries, want 2", len(got))
	}
	if got["p1"].Port != 1234 || got["p1"].Status != "ready" {
		t.Errorf("p1 roundtrip = %+v", got["p1"])
	}
	if got["p2"].Message != "connecting" {
		t.Errorf("p2 roundtrip = %+v", got["p2"])
	}
}

func TestLoadStateMissingFile(t *testing.T) {
	isolateState(t)
	got := loadState()
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
	saveState(map[string]sftpEntry{
		"p": {Port: 1234, PID: 99, Status: "ready"},
	})
	err := Stop("p")
	if err == nil || !strings.Contains(err.Error(), "lacks start-time/exe identity") {
		t.Errorf("Stop bare PID = %v, want refusal", err)
	}
	// State should be cleaned up even though we refused to signal.
	after := loadState()
	if _, ok := after["p"]; ok {
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
	st := loadState()
	if st["p"].Status != "ready" || st["p"].Port != 9000 || st["p"].Fingerprint != "fp123" {
		t.Errorf("markReady = %+v", st["p"])
	}
	RemoveState("p")
	if _, ok := loadState()["p"]; ok {
		t.Error("RemoveState should delete entry")
	}
}
