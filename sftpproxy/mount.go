package sftpproxy

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/sftp"

	"golang.org/x/crypto/ssh"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sshclient"
	"qssh/store"
)

// --- State file ---

type sftpEntry struct {
	Port        int    `json:"port"`
	PID         int    `json:"pid"`
	StartTime   uint64 `json:"start_time,omitempty"`
	Exe         string `json:"exe,omitempty"`
	URL         string `json:"url"`
	Status      string `json:"status"` // "starting", "ready", "failed"
	Message     string `json:"message,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func statePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "sftp.json")
}

var stateMu sync.Mutex

func loadState() map[string]sftpEntry {
	stateMu.Lock()
	defer stateMu.Unlock()
	data, err := os.ReadFile(statePath())
	if err != nil {
		return map[string]sftpEntry{}
	}
	var m map[string]sftpEntry
	json.Unmarshal(data, &m)
	if m == nil {
		m = make(map[string]sftpEntry)
	}
	return m
}

func saveState(m map[string]sftpEntry) {
	stateMu.Lock()
	defer stateMu.Unlock()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	path := statePath()
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0700)
	tmp, err := os.CreateTemp(dir, ".sftp-*.tmp")
	if err != nil {
		return
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
		return
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		return
	}
	cleanup = false
}

// configDir returns the qssh config directory path.
func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "qssh")
}

// --- Start (launcher, foreground) ---

// Start forks a daemon to serve SFTP and exits immediately.
// If port is 0, picks a random available port.
// bindAddr must be loopback unless allowRemote is true.
func Start(name, bindAddr string, port int, allowRemote bool) error {
	if err := ValidateBindAddr(bindAddr, allowRemote); err != nil {
		return err
	}

	state := loadState()
	if _, exists := state[name]; exists {
		return fmt.Errorf("profile %q is already running", name)
	}

	// Use specified port or pick a random one.
	portStr := "0"
	if port > 0 {
		portStr = fmt.Sprintf("%d", port)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddr, portStr))
	if err != nil {
		return fmt.Errorf("listen port: %w", err)
	}
	port = listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Write initial state before forking.
	sftpURL := fmt.Sprintf("sftp://%s:%d", bindAddr, port)
	state[name] = sftpEntry{
		Port:    port,
		URL:     sftpURL,
		Status:  "starting",
		Message: i18n.T("sftp.preparing"),
	}
	saveState(state)

	// Fork daemon — re-exec self with hidden flag.
	// Pass --sftp-allow-remote so CLI-only authorization is not lost
	// (child previously only re-read sftp.allow_non_loopback from config).
	args := []string{"--sftp-daemon", name, "--daemon-port", fmt.Sprintf("%d", port), "--bind-addr", bindAddr}
	if allowRemote {
		args = append(args, "--sftp-allow-remote")
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.SysProcAttr = daemonSysProcAttr()
	cmd.Stderr = nil // detach stderr
	cmd.Stdout = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		delete(state, name)
		saveState(state)
		return fmt.Errorf("fork daemon: %w", err)
	}

	// Wait for daemon to reach "ready".
	lastMsg := ""
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		st := loadState()
		entry, ok := st[name]
		if !ok {
			break // cleaned up — daemon likely failed
		}
		if entry.Message != "" && entry.Message != lastMsg {
			fmt.Fprintln(os.Stderr, "  → "+entry.Message)
			lastMsg = entry.Message
		}
		switch entry.Status {
		case "ready":
			fmt.Printf("SFTP proxy: %s\n", sftpURL)
			if entry.Fingerprint != "" {
				fmt.Fprintf(os.Stderr, "  SSH fingerprint: %s\n", entry.Fingerprint)
			}
			if entry.Message != "" {
				fmt.Fprintln(os.Stderr, "  "+entry.Message)
			}
			return nil
		case "failed":
			return fmt.Errorf("daemon failed")
		}
	}

	// Timeout — clean up orphaned daemon and its state.
	st := loadState()
	if entry, ok := st[name]; ok {
		id := internal.ProcessIdentity{PID: entry.PID, StartTime: entry.StartTime, Exe: entry.Exe}
		if err := internal.MatchIdentity(id); err == nil {
			_ = internal.GracefulStopIdent(id)
		}
		delete(st, name)
		saveState(st)
	}
	return fmt.Errorf("daemon did not become ready in time")
}

// --- SftpDaemon (background worker) ---

func SftpDaemon(profileName, portStr, bindAddr string, allowRemote bool) {
	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		os.Exit(1)
	}

	// Defense in depth: re-check bind address. allowRemote comes from the
	// CLI flag the parent passed through (or config). Config can only widen
	// further if set, never override a false CLI when we want fail-closed —
	// so OR config true into allowRemote.
	if cfg := internal.OpenConfig(internal.DefaultConfigPath()); cfg != nil {
		if err := cfg.LoadError(); err != nil {
			// Corrupt config: do not widen allowRemote.
		} else {
			v := strings.ToLower(strings.TrimSpace(cfg.Get("sftp.allow_non_loopback")))
			if v == "true" || v == "1" || v == "yes" {
				allowRemote = true
			}
		}
	}
	if err := ValidateBindAddr(bindAddr, allowRemote); err != nil {
		fmt.Fprintf(os.Stderr, "[sftp-daemon] %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}

	setProgress(profileName, i18n.T("sftp.opening_store"))
	openStore, err := openStoreFn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[sftp-daemon] open store: %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}

	p, exists := openStore.Get(profileName)
	if !exists {
		fmt.Fprintf(os.Stderr, "[sftp-daemon] profile %q not found\n", profileName)
		setFailed(profileName)
		os.Exit(1)
	}

	setProgress(profileName, i18n.T("sftp.connecting"))
	session, err := sshclient.DialProfile(p, openStore.Get, internal.NopProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[sftp-daemon] SSH dial: %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}
	defer session.Close()

	setProgress(profileName, i18n.T("sftp.starting"))
	sfClient, err := sftp.NewClient(session.Client())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[sftp-daemon] SFTP: %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}
	defer sfClient.Close()

	// Load/generate SSH host key.
	signer, err := loadOrGenerateHostKey(configDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[sftp-daemon] host key: %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}

	// Start SFTP proxy server.
	setProgress(profileName, i18n.T("sftp.starting_proxy"))
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddr, portStr))
	if err != nil {
		setFailed(profileName)
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- StartSFTPServer(sfClient, listener, signer)
	}()

	// Mark ready.
	sftpURL := fmt.Sprintf("sftp://%s:%d", bindAddr, port)
	fingerprint := ssh.FingerprintSHA256(signer.PublicKey())
	markReady(profileName, port, sftpURL, internal.CurrentIdentity(), fingerprint)

	// Handle SIGTERM for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-sigCh:
		listener.Close()
	case <-errCh:
		// Server exited on its own — should not happen normally.
	}

	os.Exit(0)
}

// --- Stop ---

func Stop(name string) error {
	state := loadState()
	entry, exists := state[name]
	if !exists {
		return fmt.Errorf("profile %q is not running", name)
	}

	id := internal.ProcessIdentity{
		PID:       entry.PID,
		StartTime: entry.StartTime,
		Exe:       entry.Exe,
	}
	// Refuse to signal a bare PID left over from a pre-identity sftp.json.
	// A stale/recycled PID could belong to any user process; only clean
	// state and leave any real process alone.
	if id.StartTime == 0 && id.Exe == "" {
		delete(state, name)
		saveState(state)
		return fmt.Errorf("sftp state for %q lacks start-time/exe identity; refusing to signal bare PID %d", name, id.PID)
	}
	// Kill daemon — validate full identity before signaling.
	// If the process is already gone, treat as success and clean state.
	// If identity mismatches (PID reuse), refuse to kill and still clear
	// our state (stale entry) but report the mismatch.
	var killErr error
	if entry.PID > 1 {
		if err := internal.MatchIdentity(id); err != nil {
			// Process gone or reused — do not signal a wrong process.
			killErr = err
		} else if err := internal.GracefulStopIdent(id); err != nil {
			killErr = err
		}
	}

	delete(state, name)
	saveState(state)
	// If kill failed because process is gone, that is success for Stop.
	if killErr != nil {
		// "not reachable" / starttime mismatch after exit are fine.
		msg := killErr.Error()
		if containsAny(msg, "not reachable", "no such process", "process already finished", "The system cannot find") {
			return nil
		}
		// PID reuse: state cleaned, no wrong process killed.
		if containsAny(msg, "starttime mismatch", "exe mismatch") {
			return nil
		}
		// Real kill failure (e.g. permission, terminate failed) — report.
		return fmt.Errorf("stop sftp proxy: %w", killErr)
	}
	return nil
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// --- Exported state helpers (used by daemon SFTP path) ---

// SaveState writes an SFTP entry to the state file.
func SaveState(name string, port int, bindAddr string, id internal.ProcessIdentity, fingerprint string) {
	url := fmt.Sprintf("sftp://%s:%d", bindAddr, port)
	markReady(name, port, url, id, fingerprint)
}

// RemoveState removes an SFTP entry from the state file.
func RemoveState(name string) {
	state := loadState()
	delete(state, name)
	saveState(state)
}

// --- Internal helpers ---

func markReady(name string, port int, url string, id internal.ProcessIdentity, fingerprint string) {
	if id.PID == 0 {
		id = internal.CurrentIdentity()
	}
	state := loadState()
	state[name] = sftpEntry{
		Port:        port,
		PID:         id.PID,
		StartTime:   id.StartTime,
		Exe:         id.Exe,
		URL:         url,
		Status:      "ready",
		Fingerprint: fingerprint,
	}
	saveState(state)
}

func setProgress(name, msg string) {
	state := loadState()
	if entry, ok := state[name]; ok {
		entry.Message = msg
		state[name] = entry
		saveState(state)
	}
}

func setFailed(name string) {
	state := loadState()
	delete(state, name)
	saveState(state)
}

// openStoreFn is a package-level hook so the daemon can open the store
// without importing the cmd package (would create a cycle).
// Set once at startup.
var openStoreFn func() (*store.Store, error) = nil

// SetOpenStore provides the store-opener function from cmd package.
func SetOpenStore(fn func() (*store.Store, error)) {
	openStoreFn = fn
}
