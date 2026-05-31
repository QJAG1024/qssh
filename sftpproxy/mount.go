package sftpproxy

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	data, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(statePath(), data, 0600)
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
func Start(name, bindAddr string) error {
	state := loadState()
	if _, exists := state[name]; exists {
		return fmt.Errorf("profile %q is already running", name)
	}

	// Pick a random port.
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddr, "0"))
	if err != nil {
		return fmt.Errorf("pick port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
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
	cmd := exec.Command(os.Args[0], "--sftp-daemon", name, "--daemon-port", fmt.Sprintf("%d", port), "--bind-addr", bindAddr)
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
		if proc, err := os.FindProcess(entry.PID); err == nil {
			proc.Kill()
		}
		delete(st, name)
		saveState(st)
	}
	return fmt.Errorf("daemon did not become ready in time")
}

// --- SftpDaemon (background worker) ---

func SftpDaemon(profileName, portStr, bindAddr string) {
	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
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
	session, err := sshclient.Dial(p, internal.NopProgress)
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
	markReady(profileName, port, sftpURL, os.Getpid(), fingerprint)

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

	// Kill daemon.
	proc, err := os.FindProcess(entry.PID)
	if err == nil {
		proc.Signal(syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		proc.Kill()
	}

	delete(state, name)
	saveState(state)
	return nil
}

// --- Exported state helpers (used by daemon SFTP path) ---

// SaveState writes an SFTP entry to the state file.
func SaveState(name string, port int, bindAddr string, pid int, fingerprint string) {
	url := fmt.Sprintf("sftp://%s:%d", bindAddr, port)
	markReady(name, port, url, pid, fingerprint)
}

// RemoveState removes an SFTP entry from the state file.
func RemoveState(name string) {
	state := loadState()
	delete(state, name)
	saveState(state)
}

// --- Internal helpers ---

func markReady(name string, port int, url string, pid int, fingerprint string) {
	state := loadState()
	state[name] = sftpEntry{
		Port:        port,
		PID:         pid,
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