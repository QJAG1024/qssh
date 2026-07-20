package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sftpproxy"
)

func init() {
	// Wire up store opener for SFTP daemon.
	sftpproxy.SetOpenStore(openStore)
}

// SftpStart starts an SFTP proxy for the given profile.
// Non-loopback bindAddr requires allowRemote (flag or sftp.allow_non_loopback).
func SftpStart(name, bindAddr string, port int, allowRemote bool) {
	if err := sftpproxy.ValidateBindAddr(bindAddr, allowRemote); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("sftp.failed")+"\n", err)
		os.Exit(1)
	}

	// If a daemon is already running, ask it to start SFTP proxy.
	// Daemon path rejects non-loopback; only fork-based path honors allowRemote.
	if daemonRunning(name) {
		if allowRemote && bindAddr != "" && bindAddr != "127.0.0.1" && bindAddr != "::1" && bindAddr != "localhost" {
			// Fall through to fork path so --sftp-allow-remote works with a live daemon.
		} else {
			port, fingerprint, daemonID, err := sftpViaDaemon(name, bindAddr, port)
			if err != nil {
				fmt.Fprintf(os.Stderr, i18n.T("sftp.failed")+"\n", err)
				os.Exit(1)
			}
			sftpURL := fmt.Sprintf("sftp://%s:%d", bindAddr, port)
			fmt.Printf("SFTP proxy: %s\n", sftpURL)
			if fingerprint != "" {
				fmt.Fprintf(os.Stderr, "  SSH fingerprint: %s\n", fingerprint)
			}
			// Record the daemon's identity (not this client) so --sftp-stop can
			// fall back to a safe PID kill when the daemon socket is unavailable.
			if daemonID.PID == 0 {
				daemonID = internal.CurrentIdentity()
			}
			sftpproxy.SaveState(name, port, bindAddr, daemonID, fingerprint)
			return
		}
	}

	// No daemon (or remote bind with allowRemote) — use the fork-based approach.
	if err := sftpproxy.Start(name, bindAddr, port, allowRemote); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("sftp.failed")+"\n", err)
		os.Exit(1)
	}
}

// SftpDaemon is the hidden entry point for the SFTP proxy worker.
func SftpDaemon(name, port, bindAddr string, allowRemote bool) {
	sftpproxy.SftpDaemon(name, port, bindAddr, allowRemote)
}

// SftpStop stops the SFTP proxy for a profile.
// Tries the daemon socket first, falls back to killing by PID from state file.
func SftpStop(name string) {
	// Try socket first.
	if daemonRunning(name) {
		conn, err := dialDaemon(name)
		if err == nil {
			defer conn.Close()
			data, _ := json.Marshal(daemonReq{Type: "unmount"})
			conn.Write(append(data, '\n'))

			var resp daemonResp
			if err := json.NewDecoder(conn).Decode(&resp); err == nil && resp.Type == "unmounted" {
				internal.RenderProgress(internal.StepResult{
					ID: internal.StepShellStart, Status: internal.StepDone,
					Message: i18n.T("sftp.stopped"),
				})
				sftpproxy.RemoveState(name)
				return
			}
		}
	}

	// Fall back to state-file approach.
	if err := sftpproxy.Stop(name); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("sftp.stop_failed")+"\n", err)
		os.Exit(1)
	}
	internal.RenderProgress(internal.StepResult{
		ID: internal.StepShellStart, Status: internal.StepDone,
		Message: i18n.T("sftp.stopped"),
	})
}
