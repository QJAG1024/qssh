package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
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
// Returns the exit code.
func execViaDaemon(profile, cmd string) (int, error) {
	conn, err := dialDaemon(profile)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	// Send exec request.
	req := daemonReq{Type: "exec", Cmd: cmd}
	data, _ := json.Marshal(req)
	conn.Write(append(data, '\n'))

	// Read response frames.
	dec := json.NewDecoder(conn)
	for {
		var resp daemonResp
		if err := dec.Decode(&resp); err != nil {
			if err == io.EOF {
				return -1, nil
			}
			return -1, err
		}

		switch resp.Type {
		case "stdout":
			os.Stdout.Write([]byte(resp.Data))
		case "stderr":
			os.Stderr.Write([]byte(resp.Data))
		case "exit":
			return resp.Code, nil
		case "error":
			fmt.Fprintln(os.Stderr, resp.Msg)
			return -1, nil
		}
	}
}

// sftpViaDaemon asks the daemon to start SFTP proxy, returns port and fingerprint.
func sftpViaDaemon(profile, bindAddr string) (port int, fingerprint string, err error) {
	conn, err := dialDaemon(profile)
	if err != nil {
		return 0, "", err
	}
	defer conn.Close()

	req := daemonReq{Type: "mount", BindAddr: bindAddr}
	data, _ := json.Marshal(req)
	conn.Write(append(data, '\n'))

	dec := json.NewDecoder(conn)
	var resp daemonResp
	if err := dec.Decode(&resp); err != nil {
		return 0, "", err
	}

	if resp.Type == "error" {
		return 0, "", fmt.Errorf("%s", resp.Msg)
	}
	if resp.Type != "mounted" {
		return 0, "", fmt.Errorf("unexpected response: %s", resp.Type)
	}
	return resp.Port, resp.Fingerprint, nil
}

// stopDaemon tells the daemon to shutdown.
func stopDaemon(profile string) error {
	conn, err := dialDaemon(profile)
	if err != nil {
		return err
	}
	defer conn.Close()

	req := daemonReq{Type: "stop"}
	data, _ := json.Marshal(req)
	conn.Write(append(data, '\n'))

	dec := json.NewDecoder(conn)
	var resp daemonResp
	if err := dec.Decode(&resp); err != nil {
		return err
	}

	if resp.Type == "error" {
		return fmt.Errorf("%s", resp.Msg)
	}
	if resp.Type == "stopped" && resp.Msg != "" {
		return fmt.Errorf("%s", resp.Msg)
	}
	return nil
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

// forkDaemon starts a daemon process for the given profile.
func forkDaemon(profile string, mode string) error {
	cmd := os.Args[0]
	proc, err := os.StartProcess(cmd, []string{
		cmd, "--daemon-run", profile, "--daemon-mode", string(mode),
	}, &os.ProcAttr{
		Files: []*os.File{nil, nil, nil},
	})
	if err != nil {
		return fmt.Errorf("fork daemon: %w", err)
	}
	proc.Release() // don't wait for it
	return nil
}