//go:build !windows

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/sftp"

	"qssh/internal"
	"qssh/sftpproxy"
	"qssh/sshclient"
	"qssh/store"

	"golang.org/x/crypto/ssh"
)

// --- Socket paths ---

func daemonSocketPath(profile string) string {
	d, _ := os.UserConfigDir()
	if d == "" {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "qssh", profile+".sock")
}

func daemonPidPath(profile string) string {
	d, _ := os.UserConfigDir()
	if d == "" {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "qssh", profile+".pid")
}

// --- Wire protocol (JSON lines, newline-delimited) ---

type daemonReq struct {
	Type      string   `json:"type"`                 // "exec","stdin","stdin_eof","mount","unmount","stop","ping"
	Cmd       string   `json:"cmd,omitempty"`        // legacy shell command string for exec
	Args      []string `json:"args,omitempty"`       // raw argv for exec (preferred; shell-quoted remotely)
	Data      string   `json:"data,omitempty"`       // base64 stdin chunk
	BindAddr  string   `json:"bind_addr,omitempty"`  // for mount
	MountPort int      `json:"mount_port,omitempty"` // for mount (0 = random)
}

type daemonResp struct {
	Type string `json:"type"` // "stdout","stderr","exit","mounted","error","stopped","ping"
	// Data is base64-encoded for stdout/stderr stream frames (binary-safe).
	// Control messages leave Data empty and use Msg/Code/Port instead.
	Data string `json:"data,omitempty"`
	Code int    `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
	// mount response
	Port        int    `json:"port,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Pid         int    `json:"pid,omitempty"`
}

// --- Daemon state ---

type daemonMode string

const (
	daemonPersistent daemonMode = "persistent"
	daemonManaged    daemonMode = "managed"
)

type connState struct {
	pid int
	cmd string // current command, empty when idle
}

// connWriter serializes all writes on a single client connection.
// stdout/stderr streamers and control replies share the same net.Conn.
type connWriter struct {
	mu   sync.Mutex
	conn net.Conn
}

func newConnWriter(conn net.Conn) *connWriter {
	return &connWriter{conn: conn}
}

func (w *connWriter) writeJSON(resp daemonResp) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.conn.Write(data)
	return err
}

type daemon struct {
	profile string
	mode    daemonMode
	store   *store.Store
	// profileData is a snapshot used for SetEnv etc. during exec.
	profileData store.Profile

	// sshMu guards the live SSH session/client used by exec and SFTP.
	sshMu     sync.Mutex
	session   *sshclient.Session
	sshClient *ssh.Client

	mu            sync.Mutex
	activeConns   map[string]*connState
	sftpRunning   bool
	sftpPort      int
	sftpListener  net.Listener
	idleTimeout   time.Duration
	idleTimer     *time.Timer
	stopKeepalive chan struct{}
	stopOnce      sync.Once
}

const sshKeepaliveInterval = 30 * time.Second

// --- Daemon lifecycle ---

func RunDaemon(profile string, modeStr string) {
	mode := daemonMode(modeStr)
	st, err := openStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p, exists := st.Get(profile)
	if !exists {
		fmt.Fprintf(os.Stderr, "profile %q not found\n", profile)
		os.Exit(1)
	}

	session, err := sshclient.DialProfile(p, st.Get, internal.NopProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}

	sockPath := daemonSocketPath(profile)
	os.Remove(sockPath)
	if err := os.MkdirAll(filepath.Dir(sockPath), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
	// Restrict the control socket to the owning user only.
	if err := os.Chmod(sockPath, 0600); err != nil {
		listener.Close()
		os.Remove(sockPath)
		fmt.Fprintf(os.Stderr, "chmod socket: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		listener.Close()
		os.Remove(sockPath)
		os.Remove(daemonPidPath(profile))
	}()

	os.WriteFile(daemonPidPath(profile), []byte(fmt.Sprintf("%d", os.Getpid())), 0600)

	d := &daemon{
		profile:       profile,
		profileData:   p,
		mode:          mode,
		store:         st,
		session:       session,
		sshClient:     session.Client(),
		activeConns:   make(map[string]*connState),
		stopKeepalive: make(chan struct{}),
	}
	defer d.closeSSH()

	if mode == daemonManaged {
		d.idleTimeout = 5 * time.Minute
		d.idleTimer = time.AfterFunc(d.idleTimeout, func() {
			d.mu.Lock()
			n := len(d.activeConns)
			sftp := d.sftpRunning
			d.mu.Unlock()
			if n == 0 && !sftp {
				d.stop()
				os.Exit(0)
			}
		})
	}

	go d.keepaliveLoop()

	connID := 0
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		if err := authorizePeer(conn); err != nil {
			conn.Close()
			continue
		}
		id := fmt.Sprintf("conn-%d", connID)
		connID++
		go d.handleConn(id, conn)
	}
}

// closeSSH tears down the current SSH session and jump hops.
func (d *daemon) closeSSH() {
	d.sshMu.Lock()
	defer d.sshMu.Unlock()
	d.invalidateLocked()
}

func (d *daemon) invalidateLocked() {
	if d.session != nil {
		d.session.Close()
	}
	d.session = nil
	d.sshClient = nil
}

// redialLocked opens a fresh SSH connection. Caller must hold sshMu.
// Refuses while SFTP is mounted — that path still holds the old client.
func (d *daemon) redialLocked() (*ssh.Client, error) {
	d.mu.Lock()
	sftp := d.sftpRunning
	d.mu.Unlock()
	if sftp {
		return nil, fmt.Errorf("ssh connection dead while SFTP is mounted; unmount and retry")
	}

	d.invalidateLocked()

	p, ok := d.store.Get(d.profile)
	if !ok {
		return nil, fmt.Errorf("profile %q not found", d.profile)
	}
	session, err := sshclient.DialProfile(p, d.store.Get, internal.NopProgress)
	if err != nil {
		return nil, err
	}
	d.session = session
	d.sshClient = session.Client()
	return d.sshClient, nil
}

// client returns a live SSH client, redialing once if needed.
func (d *daemon) client() (*ssh.Client, error) {
	d.sshMu.Lock()
	defer d.sshMu.Unlock()
	if d.sshClient != nil {
		return d.sshClient, nil
	}
	return d.redialLocked()
}

// newSSHSession opens a session, reconnecting once on failure.
func (d *daemon) newSSHSession() (*ssh.Session, error) {
	client, err := d.client()
	if err != nil {
		return nil, err
	}
	s, err := client.NewSession()
	if err == nil {
		return s, nil
	}
	// Connection likely dropped — reconnect once and retry.
	d.sshMu.Lock()
	if d.sshClient == client {
		d.invalidateLocked()
	}
	client, err2 := d.redialLocked()
	d.sshMu.Unlock()
	if err2 != nil {
		return nil, fmt.Errorf("ssh session: %v; reconnect: %w", err, err2)
	}
	return client.NewSession()
}

// keepaliveLoop sends OpenSSH keepalives so idle NAT/firewall paths stay up
// and dead connections are detected before the next --exec.
func (d *daemon) keepaliveLoop() {
	ticker := time.NewTicker(sshKeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopKeepalive:
			return
		case <-ticker.C:
			d.sshMu.Lock()
			client := d.sshClient
			d.sshMu.Unlock()
			if client == nil {
				// Opportunistic reconnect when idle and no SFTP.
				d.sshMu.Lock()
				if d.sshClient == nil {
					_, _ = d.redialLocked()
				}
				d.sshMu.Unlock()
				continue
			}
			// WantReply=true so a dead peer surfaces as an error.
			ok, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil || !ok {
				d.sshMu.Lock()
				if d.sshClient == client {
					d.invalidateLocked()
				}
				d.sshMu.Unlock()
			}
		}
	}
}

func (d *daemon) handleConn(id string, conn net.Conn) {
	defer conn.Close()

	pid, _ := peerPID(conn)
	w := newConnWriter(conn)

	d.mu.Lock()
	d.activeConns[id] = &connState{pid: pid}
	d.mu.Unlock()

	if d.mode == daemonManaged && d.idleTimer != nil {
		d.idleTimer.Stop()
	}

	defer func() {
		d.mu.Lock()
		delete(d.activeConns, id)
		remaining := len(d.activeConns)
		d.mu.Unlock()

		if d.mode == daemonManaged && d.idleTimer != nil && remaining == 0 {
			d.idleTimer.Reset(d.idleTimeout)
		}
	}()

	dec := json.NewDecoder(conn)
	for {
		var req daemonReq
		if err := dec.Decode(&req); err != nil {
			return // client disconnected
		}
		switch req.Type {
		case "exec":
			d.handleExec(id, req, w, dec)
		case "mount":
			d.handleMount(w, req)
		case "unmount":
			d.handleUnmount(w)
		case "stop":
			d.handleStop(w)
		case "ping":
			_ = w.writeJSON(daemonResp{Type: "ping"})
		case "stdin", "stdin_eof":
			// stdin frames outside of an active exec are ignored
		default:
			_ = w.writeJSON(daemonResp{Type: "error", Msg: "unknown type: " + req.Type})
		}
	}
}

// --- Exec ---

func (d *daemon) handleExec(id string, req daemonReq, w *connWriter, dec *json.Decoder) {
	cmd := buildRemoteCommand(req)
	if cmd == "" {
		_ = w.writeJSON(daemonResp{Type: "error", Msg: "empty command"})
		return
	}

	d.mu.Lock()
	if cs, ok := d.activeConns[id]; ok {
		cs.cmd = cmd
	}
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		if cs, ok := d.activeConns[id]; ok {
			cs.cmd = ""
		}
		d.mu.Unlock()
	}()

	sshSesh, err := d.newSSHSession()
	if err != nil {
		_ = w.writeJSON(daemonResp{Type: "error", Msg: err.Error()})
		return
	}
	defer sshSesh.Close()

	// Apply profile SetEnv before starting the remote command.
	sshclient.ApplySessionEnv(sshSesh, d.profileData)

	stdin, err := sshSesh.StdinPipe()
	if err != nil {
		_ = w.writeJSON(daemonResp{Type: "error", Msg: "stdin pipe: " + err.Error()})
		return
	}
	stdout, _ := sshSesh.StdoutPipe()
	stderr, _ := sshSesh.StderrPipe()

	if err := sshSesh.Start(cmd); err != nil {
		stdin.Close()
		// Start can also fail if the connection died between NewSession and Start.
		_ = w.writeJSON(daemonResp{Type: "error", Msg: err.Error()})
		return
	}

	// Pump client stdin frames into the remote session until eof.
	// Runs on this goroutine so it owns the decoder for the duration of exec.
	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		defer stdin.Close()
		for {
			var in daemonReq
			if err := dec.Decode(&in); err != nil {
				return
			}
			switch in.Type {
			case "stdin":
				if in.Data == "" {
					continue
				}
				payload, err := base64.StdEncoding.DecodeString(in.Data)
				if err != nil {
					// Accept raw for robustness.
					payload = []byte(in.Data)
				}
				if _, err := stdin.Write(payload); err != nil {
					return
				}
			case "stdin_eof":
				return
			default:
				// Unexpected control frame mid-exec: stop stdin, leave rest.
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamOutput(w, stdout, "stdout") }()
	go func() { defer wg.Done(); streamOutput(w, stderr, "stderr") }()
	wg.Wait()
	<-stdinDone

	code := 0
	if err := sshSesh.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			code = exitErr.ExitStatus()
		} else {
			code = 1
		}
	}
	_ = w.writeJSON(daemonResp{Type: "exit", Code: code})
}

// buildRemoteCommand prefers Args over legacy Cmd.
// Multiple args are shell-quoted (safe argv). A single arg is treated as a
// full shell command string so `qssh --exec h 'echo hi'` keeps working.
func buildRemoteCommand(req daemonReq) string {
	switch len(req.Args) {
	case 0:
		return req.Cmd
	case 1:
		return req.Args[0]
	default:
		return shellJoin(req.Args)
	}
}

// shellJoin quotes each argument for a POSIX-like remote shell.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps s in single quotes, escaping embedded quotes as '\''.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Fast path: no metacharacters that need quoting.
	safe := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '@' || c == '+' || c == ',' {
			continue
		}
		safe = false
		break
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func streamOutput(w *connWriter, r io.Reader, streamType string) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			// Base64 keeps arbitrary binary (and embedded newlines) intact
			// inside JSON-line frames; writeJSON serializes concurrent writers.
			_ = w.writeJSON(daemonResp{
				Type: streamType,
				Data: base64.StdEncoding.EncodeToString(buf[:n]),
			})
		}
		if err != nil {
			return
		}
	}
}

// --- Mount ---

func (d *daemon) handleMount(w *connWriter, req daemonReq) {
	d.mu.Lock()
	if d.sftpRunning {
		port := d.sftpPort
		d.mu.Unlock()
		_ = w.writeJSON(daemonResp{
			Type:        "mounted",
			Port:        port,
			Fingerprint: "",
			Pid:         os.Getpid(),
		})
		return
	}
	d.mu.Unlock()

	bindAddr := req.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}

	portStr := "0"
	if req.MountPort > 0 {
		portStr = fmt.Sprintf("%d", req.MountPort)
	}

	// Read config dir for host key.
	cfgDir := configDir()

	signer, err := sftpproxy.LoadHostKey(cfgDir)
	if err != nil {
		_ = w.writeJSON(daemonResp{Type: "error", Msg: "host key: " + err.Error()})
		return
	}

	fingerprint := ssh.FingerprintSHA256(signer.PublicKey())

	// Listen on random port.
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddr, portStr))
	if err != nil {
		_ = w.writeJSON(daemonResp{Type: "error", Msg: "listen: " + err.Error()})
		return
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Create sftp client from existing SSH connection (reconnect if needed).
	sshClient, err := d.client()
	if err != nil {
		listener.Close()
		_ = w.writeJSON(daemonResp{Type: "error", Msg: "ssh: " + err.Error()})
		return
	}
	sfClient, err := sftp.NewClient(sshClient)
	if err != nil {
		// Try one reconnect if the session is stale.
		d.sshMu.Lock()
		if d.sshClient == sshClient {
			d.invalidateLocked()
		}
		sshClient, err2 := d.redialLocked()
		d.sshMu.Unlock()
		if err2 != nil {
			listener.Close()
			_ = w.writeJSON(daemonResp{Type: "error", Msg: "sftp: " + err.Error()})
			return
		}
		sfClient, err = sftp.NewClient(sshClient)
		if err != nil {
			listener.Close()
			_ = w.writeJSON(daemonResp{Type: "error", Msg: "sftp: " + err.Error()})
			return
		}
	}

	d.mu.Lock()
	d.sftpRunning = true
	d.sftpPort = port
	d.sftpListener = listener
	d.mu.Unlock()

	go func() {
		sftpproxy.StartSFTPServer(sfClient, listener, signer)
		sfClient.Close()
		d.mu.Lock()
		d.sftpRunning = false
		d.sftpListener = nil
		d.mu.Unlock()
	}()

	_ = w.writeJSON(daemonResp{
		Type:        "mounted",
		Port:        port,
		Fingerprint: fingerprint,
		Pid:         os.Getpid(),
	})
}

func (d *daemon) handleUnmount(w *connWriter) {
	d.mu.Lock()
	sftpRunning := d.sftpRunning
	listener := d.sftpListener
	d.mu.Unlock()

	if !sftpRunning || listener == nil {
		_ = w.writeJSON(daemonResp{Type: "error", Msg: "no active mount"})
		return
	}

	// Close SFTP listener, the goroutine cleans up sftpRunning/sftpListener.
	listener.Close()
	_ = w.writeJSON(daemonResp{Type: "unmounted"})

	if d.mode != daemonPersistent {
		// Managed daemon: shut down entirely.
		d.shutdown()
	}
}

// --- Stop ---

func (d *daemon) handleStop(w *connWriter) {
	d.mu.Lock()
	active := make([]string, 0)
	for _, cs := range d.activeConns {
		if cs.cmd != "" {
			active = append(active, fmt.Sprintf("PID %d (%s)", cs.pid, cs.cmd))
		}
	}
	sftpActive := d.sftpRunning
	d.mu.Unlock()

	if len(active) > 0 {
		msg := "active commands: "
		for i, s := range active {
			if i > 0 {
				msg += ", "
			}
			msg += s
		}
		_ = w.writeJSON(daemonResp{Type: "stopped", Msg: msg})
		return
	}

	if sftpActive && d.mode == daemonPersistent {
		_ = w.writeJSON(daemonResp{
			Type: "stopped",
			Msg:  "SFTP proxy is running (mount active), unmount first",
		})
		return
	}

	_ = w.writeJSON(daemonResp{Type: "stopped"})
	d.shutdown()
}

func (d *daemon) stop() {
	d.stopOnce.Do(func() {
		close(d.stopKeepalive)
	})
}

func (d *daemon) shutdown() {
	d.stop()
	// Send SIGTERM to self — the defer in RunDaemon cleans up.
	process, _ := os.FindProcess(os.Getpid())
	process.Signal(syscall.SIGTERM)
}

// --- Socket helpers ---



func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "qssh")
}