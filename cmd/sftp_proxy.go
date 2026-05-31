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
func SftpStart(name, bindAddr string, port int) {
	// If a daemon is already running, ask it to start SFTP proxy.
	if daemonRunning(name) {
		port, fingerprint, err := sftpViaDaemon(name, bindAddr, port)
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("sftp.failed")+"\n", err)
			os.Exit(1)
		}
		sftpURL := fmt.Sprintf("sftp://%s:%d", bindAddr, port)
		fmt.Printf("SFTP proxy: %s\n", sftpURL)
		if fingerprint != "" {
			fmt.Fprintf(os.Stderr, "  SSH fingerprint: %s\n", fingerprint)
		}
		// Write state file so --sftp-stop finds it.
		sftpproxy.SaveState(name, port, bindAddr, os.Getpid(), fingerprint)
		return
	}

	// No daemon — use the fork-based approach.
	if err := sftpproxy.Start(name, bindAddr, port); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("sftp.failed")+"\n", err)
		os.Exit(1)
	}
}

// SftpDaemon is the hidden entry point for the SFTP proxy worker.
func SftpDaemon(name, port, bindAddr string) {
	sftpproxy.SftpDaemon(name, port, bindAddr)
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