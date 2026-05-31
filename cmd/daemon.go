//go:build !windows

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/sftp"

	"qssh/internal"
	"qssh/sftpproxy"
	"qssh/sshclient"

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
	Type     string `json:"type"`               // "exec", "mount", "unmount", "stop"
	Cmd      string `json:"cmd,omitempty"`       // for exec
	BindAddr string `json:"bind_addr,omitempty"` // for mount
}

type daemonResp struct {
	Type string `json:"type"` // "stdout","stderr","exit","mounted","error","stopped","ping"
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

type daemon struct {
	profile   string
	mode      daemonMode
	sshClient *ssh.Client

	mu           sync.Mutex
	activeConns  map[string]*connState
	sftpRunning  bool
	sftpPort     int
	sftpListener net.Listener
	idleTimeout  time.Duration
	idleTimer    *time.Timer
}

func (d *daemon) writeJSON(conn net.Conn, resp daemonResp) {
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}

// --- Daemon lifecycle ---

func RunDaemon(profile string, modeStr string) {
	mode := daemonMode(modeStr)
	store, err := openStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p, exists := store.Get(profile)
	if !exists {
		fmt.Fprintf(os.Stderr, "profile %q not found\n", profile)
		os.Exit(1)
	}

	session, err := sshclient.Dial(p, internal.NopProgress)
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
	defer func() {
		listener.Close()
		os.Remove(sockPath)
		os.Remove(daemonPidPath(profile))
	}()

	os.WriteFile(daemonPidPath(profile), []byte(fmt.Sprintf("%d", os.Getpid())), 0644)

	d := &daemon{
		profile:     profile,
		mode:        mode,
		sshClient:   session.Client(),
		activeConns: make(map[string]*connState),
	}

	if mode == daemonManaged {
		d.idleTimeout = 5 * time.Minute
		d.idleTimer = time.AfterFunc(d.idleTimeout, func() {
			d.mu.Lock()
			n := len(d.activeConns)
			d.mu.Unlock()
			if n == 0 {
				os.Exit(0)
			}
		})
	}

	connID := 0
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		id := fmt.Sprintf("conn-%d", connID)
		connID++
		go d.handleConn(id, conn)
	}
}

func (d *daemon) handleConn(id string, conn net.Conn) {
	defer conn.Close()

	pid, _ := peerPID(conn)

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
			d.handleExec(id, req.Cmd, conn)
		case "mount":
			d.handleMount(conn, req)
		case "unmount":
			d.handleUnmount(conn)
		case "stop":
			d.handleStop(conn)
		case "ping":
			d.writeJSON(conn, daemonResp{Type: "ping"})
		default:
			d.writeJSON(conn, daemonResp{Type: "error", Msg: "unknown type: " + req.Type})
		}
	}
}

// --- Exec ---

func (d *daemon) handleExec(id string, cmd string, conn net.Conn) {
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

	sshSesh, err := d.sshClient.NewSession()
	if err != nil {
		d.writeJSON(conn, daemonResp{Type: "error", Msg: err.Error()})
		return
	}
	defer sshSesh.Close()

	stdout, _ := sshSesh.StdoutPipe()
	stderr, _ := sshSesh.StderrPipe()

	if err := sshSesh.Start(cmd); err != nil {
		d.writeJSON(conn, daemonResp{Type: "error", Msg: err.Error()})
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamOutput(conn, stdout, "stdout") }()
	go func() { defer wg.Done(); streamOutput(conn, stderr, "stderr") }()
	wg.Wait()

	code := 0
	if err := sshSesh.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			code = exitErr.ExitStatus()
		} else {
			code = 1
		}
	}
	d.writeJSON(conn, daemonResp{Type: "exit", Code: code})
}

func streamOutput(conn net.Conn, r io.Reader, streamType string) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			// Write each chunk as a JSON frame.
			// Large writes are batched at the net.Conn level.
			data, _ := json.Marshal(daemonResp{Type: streamType, Data: string(buf[:n])})
			data = append(data, '\n')
			conn.Write(data)
		}
		if err != nil {
			return
		}
	}
}

// --- Mount ---

func (d *daemon) handleMount(conn net.Conn, req daemonReq) {
	d.mu.Lock()
	if d.sftpRunning {
		d.mu.Unlock()
		d.writeJSON(conn, daemonResp{
			Type:        "mounted",
			Port:        d.sftpPort,
			Fingerprint: "",
		})
		return
	}
	d.mu.Unlock()

	bindAddr := req.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}

	// Read config dir for host key.
	cfgDir := configDir()

	signer, err := sftpproxy.LoadHostKey(cfgDir)
	if err != nil {
		d.writeJSON(conn, daemonResp{Type: "error", Msg: "host key: " + err.Error()})
		return
	}

	fingerprint := ssh.FingerprintSHA256(signer.PublicKey())

	// Listen on random port.
	listener, err := net.Listen("tcp", net.JoinHostPort(bindAddr, "0"))
	if err != nil {
		d.writeJSON(conn, daemonResp{Type: "error", Msg: "listen: " + err.Error()})
		return
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Create sftp client from existing SSH connection.
	sfClient, err := sftp.NewClient(d.sshClient)
	if err != nil {
		listener.Close()
		d.writeJSON(conn, daemonResp{Type: "error", Msg: "sftp: " + err.Error()})
		return
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

	d.writeJSON(conn, daemonResp{
		Type:        "mounted",
		Port:        port,
		Fingerprint: fingerprint,
		Pid:         os.Getpid(),
	})
}

func (d *daemon) handleUnmount(conn net.Conn) {
	d.mu.Lock()
	sftpRunning := d.sftpRunning
	listener := d.sftpListener
	d.mu.Unlock()

	if !sftpRunning || listener == nil {
		d.writeJSON(conn, daemonResp{Type: "error", Msg: "no active mount"})
		return
	}

	// Close SFTP listener, the goroutine cleans up sftpRunning/sftpListener.
	listener.Close()
	d.writeJSON(conn, daemonResp{Type: "unmounted"})

	if d.mode != daemonPersistent {
		// Managed daemon: shut down entirely.
		d.shutdown()
	}
}

// --- Stop ---

func (d *daemon) handleStop(conn net.Conn) {
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
		d.writeJSON(conn, daemonResp{Type: "stopped", Msg: msg})
		return
	}

	if sftpActive && d.mode == daemonPersistent {
		d.writeJSON(conn, daemonResp{
			Type: "stopped",
			Msg:  "SFTP proxy is running (mount active), unmount first",
		})
		return
	}

	d.writeJSON(conn, daemonResp{Type: "stopped"})
	d.shutdown()
}

func (d *daemon) shutdown() {
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