//go:build !windows

package internal

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestGracefulStop_KillsProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	st, err := processStartTime(pid)
	if err != nil {
		t.Fatalf("starttime: %v", err)
	}
	id := ProcessIdentity{PID: pid, StartTime: st}

	start := time.Now()
	if err := GracefulStopIdent(id); err != nil {
		t.Fatalf("GracefulStopIdent: %v", err)
	}
	elapsed := time.Since(start)
	// Wait reaped
	_ = cmd.Wait()

	if err := MatchIdentity(id); err == nil {
		t.Fatal("process still matches identity after GracefulStop")
	}
	// Should not take the full 2s if SIGTERM works immediately.
	if elapsed > 3*time.Second {
		t.Fatalf("GracefulStop took too long: %v", elapsed)
	}
	t.Logf("stopped in %v", elapsed)
}

func TestMatchIdentity_StartTimeMismatch(t *testing.T) {
	// Self should match with correct starttime.
	id := CurrentIdentity()
	if err := MatchIdentity(id); err != nil {
		t.Fatalf("self identity: %v", err)
	}
	// Wrong starttime must fail.
	id.StartTime = 1
	if id.StartTime == 0 {
		t.Skip("starttime unavailable")
	}
	// Re-read true starttime and set a fake one
	trueID := CurrentIdentity()
	if trueID.StartTime == 0 {
		t.Skip("starttime unavailable")
	}
	fake := trueID
	fake.StartTime = trueID.StartTime + 99999
	if err := MatchIdentity(fake); err == nil {
		t.Fatal("expected starttime mismatch")
	}
}

func TestSafePID_RejectsZeroOne(t *testing.T) {
	if err := SafePID(0); err == nil {
		t.Fatal("pid 0 should be rejected")
	}
	if err := SafePID(1); err == nil {
		t.Fatal("pid 1 should be rejected")
	}
	if err := SafePID(os.Getpid()); err != nil {
		t.Fatalf("self should be ok: %v", err)
	}
}
