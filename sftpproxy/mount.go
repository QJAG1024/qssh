package sftpproxy

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/sftp"

	"golang.org/x/crypto/ssh"

	"qssh/internal"
	"qssh/internal/daemonstate"
	"qssh/internal/i18n"
	"qssh/sshclient"
	"qssh/store"
)

// --- State file ---

func statePath() string {
	if p := os.Getenv("QSSH_SFTP_STATE"); p != "" {
		return p
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "sftp.json")
}

// stateFile returns a handle to this service's daemon state file. A fresh
// handle per call keeps tests that swap QSSH_SFTP_STATE per-test isolated.
func stateFile() *daemonstate.File { return daemonstate.Open(statePath()) }

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

	if _, exists := stateFile().Get(name); exists {
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
	if err := stateFile().SetEntry(name, daemonstate.Entry{
		Port:    port,
		URL:     sftpURL,
		Status:  daemonstate.StatusStarting,
		Message: i18n.T("sftp.preparing"),
	}); err != nil {
		return fmt.Errorf("write sftp state: %w", err)
	}

	// Fork daemon — re-exec self with hidden flag.
	// Pass --sftp-allow-remote so CLI-only authorization is not lost
	// (child previously only re-read sftp.allow_non_loopback from config).
	args := []string{"--sftp-daemon", name, "--daemon-port", fmt.Sprintf("%d", port), "--bind-addr", bindAddr}
	if allowRemote {
		args = append(args, "--sftp-allow-remote")
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.SysProcAttr = DaemonSysProcAttr()
	cmd.Stderr = nil // detach stderr
	cmd.Stdout = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		_ = stateFile().DeleteEntry(name)
		return fmt.Errorf("fork daemon: %w", err)
	}

	// Wait for daemon to reach "ready".
	lastMsg := ""
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		entry, ok := stateFile().Get(name)
		if !ok {
			break // cleaned up — daemon likely failed
		}
		if entry.Message != "" && entry.Message != lastMsg {
			fmt.Fprintln(os.Stderr, "  → "+entry.Message)
			lastMsg = entry.Message
		}
		switch entry.Status {
		case daemonstate.StatusReady:
			fmt.Printf(i18n.T("sftp.proxy_started")+"\n", sftpURL)
			if entry.Fingerprint != "" {
				fmt.Fprintf(os.Stderr, "  SSH fingerprint: %s\n", entry.Fingerprint)
			}
			if entry.Message != "" {
				fmt.Fprintln(os.Stderr, "  "+entry.Message)
			}
			return nil
		case daemonstate.StatusFailed:
			return fmt.Errorf("daemon failed")
		}
	}

	// Timeout — clean up orphaned daemon and its state.
	if entry, ok := stateFile().Get(name); ok {
		if err := internal.MatchIdentity(entry.Identity()); err == nil {
			_ = internal.GracefulStopIdent(entry.Identity())
		}
		_ = stateFile().DeleteEntry(name)
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
	// Concurrent writes/reads for high-latency links (uploads otherwise
	// serialize on per-packet ACKs).
	sfClient, err := sftp.NewClient(session.Client(),
		sftp.UseConcurrentWrites(true),
		sftp.UseConcurrentReads(true),
		sftp.MaxConcurrentRequestsPerFile(8),
		sftp.MaxPacket(32*1024),
	)
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
	entry, exists := stateFile().Get(name)
	if !exists {
		return fmt.Errorf("profile %q is not running", name)
	}

	id := entry.Identity()
	// Refuse to signal a bare PID left over from a pre-identity sftp.json.
	// A stale/recycled PID could belong to any user process; only clean
	// state and leave any real process alone.
	if !entry.HasIdentity() {
		_ = stateFile().DeleteEntry(name)
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

	_ = stateFile().DeleteEntry(name)
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
	_ = stateFile().DeleteEntry(name)
}

// --- Internal helpers ---

func markReady(name string, port int, url string, id internal.ProcessIdentity, fingerprint string) {
	if id.PID == 0 {
		id = internal.CurrentIdentity()
	}
	_ = stateFile().SetEntry(name, daemonstate.Entry{
		Port:        port,
		PID:         id.PID,
		StartTime:   id.StartTime,
		Exe:         id.Exe,
		URL:         url,
		Status:      daemonstate.StatusReady,
		Fingerprint: fingerprint,
	})
}

func setProgress(name, msg string) {
	_ = stateFile().SetMessage(name, msg)
}

func setFailed(name string) {
	_ = stateFile().DeleteEntry(name)
}

// openStoreFn is a package-level hook so the daemon can open the store
// without importing the cmd package (would create a cycle).
// Set once at startup.
var openStoreFn func() (*store.Store, error) = nil

// SetOpenStore provides the store-opener function from cmd package.
func SetOpenStore(fn func() (*store.Store, error)) {
	openStoreFn = fn
}

// Status returns the running SFTP proxy entries. When name is non-empty,
// returns just that profile (nil if not running).
func Status(name string) map[string]daemonstate.Entry {
	all := stateFile().All()
	if name == "" {
		return all
	}
	if e, ok := all[name]; ok {
		return map[string]daemonstate.Entry{name: e}
	}
	return nil
}
