//go:build windows

package cmd

import (
	"fmt"
	"net"
	"os"
)

// --- Types (must exist for sftp_proxy.go references) ---

type daemonReq struct {
	Type     string `json:"type"`
	Cmd      string `json:"cmd,omitempty"`
	BindAddr string `json:"bind_addr,omitempty"`
}

type daemonResp struct {
	Type        string `json:"type"`
	Data        string `json:"data,omitempty"`
	Code        int    `json:"code,omitempty"`
	Msg         string `json:"msg,omitempty"`
	Port        int    `json:"port,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Pid         int    `json:"pid,omitempty"`
}

// --- Exported ---

func RunDaemon(profile, modeStr string) {
	fmt.Fprintln(os.Stderr, "daemon is not supported on Windows")
	os.Exit(1)
}

func StartDaemon(profile string) {
	fmt.Fprintln(os.Stderr, "daemon is not supported on Windows")
	os.Exit(1)
}

func StopDaemon(profile string) {
	fmt.Fprintln(os.Stderr, "daemon is not supported on Windows")
	os.Exit(1)
}

// --- Unexported, used by exec.go / sftp_proxy.go ---

func daemonRunning(profile string) bool { return false }

func startManagedDaemon(profile string) error {
	return fmt.Errorf("not supported on Windows")
}

func execViaDaemon(profile, cmd string) (int, error) {
	return -1, fmt.Errorf("not supported on Windows")
}

func sftpViaDaemon(profile, bindAddr string) (int, string, error) {
	return 0, "", fmt.Errorf("not supported on Windows")
}

func dialDaemon(profile string) (net.Conn, error) {
	return nil, fmt.Errorf("not supported on Windows")
}

// --- Socket helpers (used by forkDaemon, which is !windows) ---
// Keep as stubs since they may be referenced in dead code paths.

func daemonSocketPath(profile string) string { return "" }
func daemonPidPath(profile string) string    { return "" }
