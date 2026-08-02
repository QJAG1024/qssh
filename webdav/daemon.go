// WebDAV daemon lifecycle: fork a child process that serves the SFTP-backed
// WebDAV endpoint, tracking it in a state file. Modeled on sftpproxy's
// Start/Stop but independent (WebDAV is a long-lived HTTP server, not a
// connection-reuse daemon).
package webdav

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"qssh/internal"
	"qssh/sftpproxy"
	"qssh/sshclient"
	"qssh/store"
)

// --- State file ---

type entry struct {
	Port      int    `json:"port"`
	PID       int    `json:"pid"`
	StartTime uint64 `json:"start_time,omitempty"`
	Exe       string `json:"exe,omitempty"`
	URL       string `json:"url"`
	Status    string `json:"status"` // "starting", "ready", "failed"
	Message   string `json:"message,omitempty"`
}

func statePath() string {
	if p := os.Getenv("QSSH_WEBDAV_STATE"); p != "" {
		return p
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "qssh", "webdav.json")
}

var stateMu sync.Mutex

func loadState() map[string]entry {
	stateMu.Lock()
	defer stateMu.Unlock()
	data, err := os.ReadFile(statePath())
	if err != nil {
		return map[string]entry{}
	}
	var m map[string]entry
	json.Unmarshal(data, &m)
	if m == nil {
		m = make(map[string]entry)
	}
	return m
}

func saveState(m map[string]entry) {
	stateMu.Lock()
	defer stateMu.Unlock()
	data, _ := json.MarshalIndent(m, "", "  ")
	path := statePath()
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0700)
	tmp, err := os.CreateTemp(dir, ".webdav-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	tmp.Chmod(0600)
	tmp.Write(data)
	tmp.Close()
	internal.ReplaceFile(tmpName, path)
}

// --- Start/Stop ---

// Start forks a WebDAV daemon for the profile. bindAddr must be loopback
// unless allowRemote is true. port 0 picks a random port.
func Start(name, bindAddr string, port int, allowRemote bool) error {
	if err := sftpproxy.ValidateBindAddr(bindAddr, allowRemote); err != nil {
		return err
	}
	st := loadState()
	if _, exists := st[name]; exists {
		return fmt.Errorf("profile %q is already running", name)
	}

	portStr := "0"
	if port > 0 {
		portStr = fmt.Sprintf("%d", port)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bindAddr, portStr))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	port = ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	url := fmt.Sprintf("http://%s:%d/", bindAddr, port)
	st[name] = entry{Port: port, URL: url, Status: "starting", Message: "starting"}
	saveState(st)

	args := []string{"--webdav-daemon", name, "--daemon-port", fmt.Sprintf("%d", port), "--bind-addr", bindAddr}
	if allowRemote {
		args = append(args, "--sftp-allow-remote")
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.SysProcAttr = sftpproxy.DaemonSysProcAttr()
	cmd.Stderr = nil
	cmd.Stdout = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		delete(st, name)
		saveState(st)
		return fmt.Errorf("fork: %w", err)
	}

	// Wait for ready.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		e := loadState()[name]
		switch e.Status {
		case "ready":
			fmt.Printf("WebDAV: %s\n", url)
			return nil
		case "failed":
			return fmt.Errorf("webdav daemon failed: %s", e.Message)
		}
	}
	return fmt.Errorf("webdav daemon did not become ready in time")
}

// Stop terminates the WebDAV daemon for the profile.
func Stop(name string) error {
	st := loadState()
	e, exists := st[name]
	if !exists {
		return fmt.Errorf("profile %q is not running", name)
	}
	id := internal.ProcessIdentity{PID: e.PID, StartTime: e.StartTime, Exe: e.Exe}
	if id.StartTime == 0 && id.Exe == "" {
		delete(st, name)
		saveState(st)
		return fmt.Errorf("webdav state for %q lacks identity; refusing to signal bare PID %d", name, e.PID)
	}
	if e.PID > 1 {
		if err := internal.MatchIdentity(id); err == nil {
			_ = internal.GracefulStopIdent(id)
		}
	}
	delete(st, name)
	saveState(st)
	return nil
}

// --- Daemon worker (--webdav-daemon) ---

// Daemon runs the WebDAV server in the child process.
func Daemon(profileName, portStr, bindAddr string, allowRemote bool) {
	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		os.Exit(1)
	}

	setProgress(profileName, "opening store")
	openStore, err := openStoreFn()
	if err != nil {
		setFailed(profileName, err.Error())
		os.Exit(1)
	}
	p, exists := openStore.Get(profileName)
	if !exists {
		setFailed(profileName, "profile not found")
		os.Exit(1)
	}

	setProgress(profileName, "connecting")
	session, err := sshclient.DialProfile(p, openStore.Get, internal.NopProgress)
	if err != nil {
		setFailed(profileName, "ssh: "+err.Error())
		os.Exit(1)
	}
	defer session.Close()

	// Concurrent writes are essential on high-latency links: without them
	// each SFTP write packet waits for an ACK (~200ms per round-trip), making
	// uploads orders of magnitude slower than downloads.
	sfClient, err := sftp.NewClient(session.Client(),
		sftp.UseConcurrentWrites(true),
		sftp.UseConcurrentReads(true),
		sftp.MaxConcurrentRequestsPerFile(8),
		sftp.MaxPacket(32*1024),
	)
	if err != nil {
		setFailed(profileName, "sftp: "+err.Error())
		os.Exit(1)
	}
	defer sfClient.Close()

	ln, err := net.Listen("tcp", net.JoinHostPort(bindAddr, portStr))
	if err != nil {
		setFailed(profileName, "listen: "+err.Error())
		os.Exit(1)
	}

	srv := New(sfClient)
	go http.Serve(ln, srv)

	url := fmt.Sprintf("http://%s:%d/", bindAddr, port)
	markReady(profileName, port, url, internal.CurrentIdentity())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
	ln.Close()
	os.Exit(0)
}

// --- state helpers ---

func markReady(name string, port int, url string, id internal.ProcessIdentity) {
	st := loadState()
	st[name] = entry{Port: port, PID: id.PID, StartTime: id.StartTime, Exe: id.Exe, URL: url, Status: "ready"}
	saveState(st)
}

func setProgress(name, msg string) {
	st := loadState()
	if e, ok := st[name]; ok {
		e.Message = msg
		st[name] = e
		saveState(st)
	}
}

func setFailed(name, msg string) {
	st := loadState()
	if e, ok := st[name]; ok {
		e.Status = "failed"
		e.Message = msg
		st[name] = e
		saveState(st)
	}
}

// openStoreFn is wired by cmd (avoids import cycle).
var openStoreFn func() (*store.Store, error)

// SetOpenStore provides the store opener from the cmd package.
func SetOpenStore(fn func() (*store.Store, error)) {
	openStoreFn = fn
}
