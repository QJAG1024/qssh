//go:build !windows

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"qssh/internal"
)

// daemonRunning checks if a daemon socket exists and responds to ping.
func daemonRunning(profile string) bool {
	sockPath := daemonSocketPath(profile)
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false
	}
	defer conn.Close()

	// Send ping.
	data, _ := json.Marshal(daemonReq{Type: "ping"})
	conn.Write(append(data, '\n'))

	// Read response.
	dec := json.NewDecoder(conn)
	var resp daemonResp
	if err := dec.Decode(&resp); err != nil {
		return false
	}
	return resp.Type == "ping"
}

// execViaDaemon sends a command to the daemon and streams results to stdout/stderr.
// Args are sent as a raw argv array (daemon shell-quotes them). Local stdin is
// forwarded as base64 stdin frames until EOF.
// Returns the exit code.
func execViaDaemon(profile string, args []string) (int, error) {
	conn, err := dialDaemon(profile)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	// Send exec request with raw argv.
	req := daemonReq{Type: "exec", Args: args, Cmd: strings.Join(args, " ")}
	data, _ := json.Marshal(req)
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return -1, err
	}

	// Stream local stdin only when it is not a TTY (pipe/redirect). Interactive
	// terminals would otherwise block forever waiting for keyboard input.
	// When the remote command finishes first, SetReadDeadline unblocks a stuck
	// stdin Read (common with empty inherited pipes that never EOF).
	stdinDone := make(chan struct{})
	cancelStdin := func() {
		_ = os.Stdin.SetReadDeadline(time.Now())
	}
	go func() {
		defer close(stdinDone)
		_ = os.Stdin.SetReadDeadline(time.Time{})
		defer func() { _ = os.Stdin.SetReadDeadline(time.Time{}) }()

		sendEOF := func() {
			frame, _ := json.Marshal(daemonReq{Type: "stdin_eof"})
			_, _ = conn.Write(append(frame, '\n'))
		}
		if term.IsTerminal(int(os.Stdin.Fd())) {
			sendEOF()
			return
		}
		buf := make([]byte, 32*1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				frame, _ := json.Marshal(daemonReq{
					Type: "stdin",
					Data: base64.StdEncoding.EncodeToString(buf[:n]),
				})
				if _, werr := conn.Write(append(frame, '\n')); werr != nil {
					return
				}
			}
			if err != nil {
				sendEOF()
				return
			}
		}
	}()

	finish := func(code int, retErr error) (int, error) {
		cancelStdin()
		_ = conn.Close()
		select {
		case <-stdinDone:
		case <-time.After(200 * time.Millisecond):
		}
		return code, retErr
	}

	// Read response frames.
	dec := json.NewDecoder(conn)
	for {
		var resp daemonResp
		if err := dec.Decode(&resp); err != nil {
			if err == io.EOF {
				return finish(-1, nil)
			}
			return finish(-1, err)
		}

		switch resp.Type {
		case "stdout", "stderr":
			// Stream frames are base64-encoded for binary safety.
			// Fall back to raw bytes for older daemons that sent plain text.
			payload, err := base64.StdEncoding.DecodeString(resp.Data)
			if err != nil {
				payload = []byte(resp.Data)
			}
			if resp.Type == "stdout" {
				os.Stdout.Write(payload)
			} else {
				os.Stderr.Write(payload)
			}
		case "exit":
			return finish(resp.Code, nil)
		case "error":
			fmt.Fprintln(os.Stderr, resp.Msg)
			return finish(-1, nil)
		}
	}
}

// sftpViaDaemon asks the daemon to start SFTP proxy.
// Returns port, fingerprint, and the daemon identity that owns the mount.
func sftpViaDaemon(profile, bindAddr string, port int) (int, string, internal.ProcessIdentity, error) {
	conn, err := dialDaemon(profile)
	if err != nil {
		return 0, "", internal.ProcessIdentity{}, err
	}
	defer conn.Close()

	req := daemonReq{Type: "mount", BindAddr: bindAddr, MountPort: port}
	data, _ := json.Marshal(req)
	conn.Write(append(data, '\n'))

	dec := json.NewDecoder(conn)
	var resp daemonResp
	if err := dec.Decode(&resp); err != nil {
		return 0, "", internal.ProcessIdentity{}, err
	}

	if resp.Type == "error" {
		return 0, "", internal.ProcessIdentity{}, fmt.Errorf("%s", resp.Msg)
	}
	if resp.Type != "mounted" {
		return 0, "", internal.ProcessIdentity{}, fmt.Errorf("unexpected response: %s", resp.Type)
	}
	id := internal.ProcessIdentity{PID: resp.Pid, StartTime: resp.StartTime, Exe: resp.Exe}
	return resp.Port, resp.Fingerprint, id, nil
}

// stopDaemon tells the daemon to shutdown.
// Uses force=true so active commands / SFTP cannot block revocation
// (delete/edit/rename must not leave an authenticated session behind).
// Falls back to SIGTERM/SIGKILL via PID file if the socket is dead.
func stopDaemon(profile string) error {
	var sockErr error
	conn, err := dialDaemon(profile)
	if err == nil {
		req := daemonReq{Type: "stop", Force: true}
		data, _ := json.Marshal(req)
		_, _ = conn.Write(append(data, '\n'))

		dec := json.NewDecoder(conn)
		var resp daemonResp
		if err := dec.Decode(&resp); err != nil {
			sockErr = err
		} else if resp.Type == "error" {
			sockErr = fmt.Errorf("%s", resp.Msg)
		} else if resp.Type == "stopped" && resp.Msg != "" {
			// Legacy non-force response carrying a refusal message.
			sockErr = fmt.Errorf("%s", resp.Msg)
		}
		conn.Close()
		if sockErr == nil {
			// Wait briefly for process exit / socket removal.
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if !daemonRunning(profile) {
					cleanupDaemonFiles(profile)
					return nil
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	} else {
		sockErr = err
	}

	// Force-kill via PID file if socket path failed or daemon still alive.
	pidErr := killDaemonByPID(profile)
	if pidErr != nil {
		// Daemon already cleaned up its own PID file -> stopped successfully.
		if os.IsNotExist(pidErr) {
			cleanupDaemonFiles(profile)
			return nil
		}
		// Legacy bare-PID file: do not risk signalling a recycled PID.
		// Clean up the stale state and report success.
		if strings.Contains(pidErr.Error(), "lacks start-time/exe identity") {
			cleanupDaemonFiles(profile)
			return nil
		}
		if sockErr != nil {
			return fmt.Errorf("stop daemon: socket: %v; pid: %w", sockErr, pidErr)
		}
		return fmt.Errorf("stop daemon: pid: %w", pidErr)
	}
	cleanupDaemonFiles(profile)
	return nil
}

func cleanupDaemonFiles(profile string) {
	_ = os.Remove(daemonSocketPath(profile))
	_ = os.Remove(daemonPidPath(profile))
}

func killDaemonByPID(profile string) error {
	data, err := os.ReadFile(daemonPidPath(profile))
	if err != nil {
		return err
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return fmt.Errorf("parse pid file: %w", err)
	}
	id := internal.ProcessIdentity{PID: pid}
	// Prefer full identity if the pid file contains starttime (pid starttime [exe]).
	fields := strings.Fields(string(data))
	if len(fields) >= 2 {
		var st uint64
		if _, err := fmt.Sscanf(fields[1], "%d", &st); err == nil {
			id.StartTime = st
		}
	}
	if len(fields) >= 3 {
		id.Exe = fields[2]
	}
	// Refuse to signal a bare PID left over from an pre-identity upgrade.
	// A stale/recycled PID could belong to any user process; only cleanup
	// files and leave any real daemon alone.
	if id.StartTime == 0 && id.Exe == "" {
		return fmt.Errorf("pid file for %q lacks start-time/exe identity; refusing to signal bare PID %d", profile, id.PID)
	}
	return internal.GracefulStopIdent(id)
}

func dialDaemon(profile string) (net.Conn, error) {
	return net.Dial("unix", daemonSocketPath(profile))
}

// StartDaemon starts a persistent daemon for a profile.
func StartDaemon(profile string) {
	if daemonRunning(profile) {
		fmt.Println("daemon is already running")
		return
	}
	if err := forkDaemon(profile, "persistent"); err != nil {
		fmt.Fprintf(os.Stderr, "start daemon: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("daemon started for %q\n", profile)
}

// StopDaemon stops a running daemon.
func StopDaemon(profile string) {
	if err := stopDaemon(profile); err != nil {
		fmt.Fprintf(os.Stderr, "stop daemon: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("daemon stopped for %q\n", profile)
}

// startManagedDaemon forks a managed daemon and waits for it to be ready.
func startManagedDaemon(profile string) error {
	if err := forkDaemon(profile, "managed"); err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if daemonRunning(profile) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start within 30s")
}

// forkDaemon starts a daemon process for the given profile.
func forkDaemon(profile string, mode string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	proc, err := os.StartProcess(exe, []string{
		exe, "--daemon-run", profile, "--daemon-mode", string(mode),
	}, &os.ProcAttr{
		Files: []*os.File{nil, nil, nil},
	})
	if err != nil {
		return fmt.Errorf("fork daemon: %w", err)
	}
	proc.Release() // don't wait for it
	return nil
}
