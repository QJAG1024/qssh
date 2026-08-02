package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sftpproxy"
)

func init() {
	// Wire up store opener for SFTP daemon.
	sftpproxy.SetOpenStore(openStore)
}

// sftpBindOrigin records where the effective SFTP bind address came from.
type sftpBindOrigin string

const (
	bindOriginCLI     sftpBindOrigin = "cli"     // --bind flag: explicit, warn+allow if non-loopback
	bindOriginProfile sftpBindOrigin = "profile" // profile sftp.bind: per-profile choice authorizes
	bindOriginGlobal  sftpBindOrigin = "global"  // global sftp.bind: needs sftp.allow_non_loopback=true
	bindOriginDefault sftpBindOrigin = "default" // 127.0.0.1
)

// resolveSFTPBind determines the effective SFTP bind address and whether a
// non-loopback bind is authorized, given the CLI --bind flag (may be empty)
// and the profile's Options map.
//
// Precedence: CLI --bind > profile sftp.bind > global sftp.bind > 127.0.0.1.
// Authorization by origin:
//   - CLI: the user explicitly passed --bind, so a non-loopback address is
//     authorized (a warning is printed by the caller before proceeding).
//   - profile: the per-profile sftp.bind value is itself the authorization —
//     configuring it for one profile is an explicit, informed choice.
//   - global: requires sftp.allow_non_loopback=true; otherwise the caller
//     refuses to start with an explanation.
func resolveSFTPBind(cliBind string, profileOpts map[string]string) (addr string, allowRemote bool, origin sftpBindOrigin) {
	if cliBind != "" {
		return cliBind, !sftpproxy.IsLoopbackAddr(cliBind), bindOriginCLI
	}
	// Check the profile's own Options map directly — EffectiveOption would
	// fall through to the global value and mislabel it as a profile choice.
	if profileOpts != nil {
		if v, ok := profileOpts["sftp.bind"]; ok && strings.TrimSpace(v) != "" {
			return v, !sftpproxy.IsLoopbackAddr(v), bindOriginProfile
		}
	}
	if cfg := internal.OpenConfig(internal.DefaultConfigPath()); cfg != nil {
		if v := strings.TrimSpace(cfg.Get("sftp.bind")); v != "" {
			nonLoop := !sftpproxy.IsLoopbackAddr(v)
			allow := false
			if nonLoop && cfg.LoadError() == nil {
				av := strings.ToLower(strings.TrimSpace(cfg.Get("sftp.allow_non_loopback")))
				allow = av == "true" || av == "1" || av == "yes"
			}
			return v, allow, bindOriginGlobal
		}
	}
	return "127.0.0.1", false, bindOriginDefault
}

// warnNonLoopback prints a 2-second warning before proceeding with a
// non-loopback bind from an explicit --bind flag.
func warnNonLoopback(addr string) {
	fmt.Fprintf(os.Stderr, i18n.T("sftp.bind.warn_cli")+"\n", addr)
	time.Sleep(2 * time.Second)
}

// SftpStart starts an SFTP proxy for the given profile.
// cliBind is the raw --bind flag value (empty = resolve profile/global/default).
func SftpStart(name, cliBind string, port int, deprecatedAllowRemote bool) {
	if deprecatedAllowRemote {
		fmt.Fprintln(os.Stderr, i18n.T("sftp.bind.deprecated_flag"))
	}

	// Load the profile once; its Options drive the bind resolution unless a
	// CLI --bind was passed explicitly (CLI wins over everything).
	var profileOpts map[string]string
	if cliBind == "" {
		st, err := openStore()
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("store.open_error")+"\n", err)
			os.Exit(1)
		}
		p, exists := st.Get(name)
		if !exists {
			fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
			os.Exit(1)
		}
		profileOpts = p.Options
	}
	bindAddr, allowRemote, origin := resolveSFTPBind(cliBind, profileOpts)

	// Origin-specific gate before validation.
	switch origin {
	case bindOriginCLI:
		if allowRemote {
			warnNonLoopback(bindAddr)
		}
	case bindOriginGlobal:
		if allowRemote {
			// authorized via sftp.allow_non_loopback=true
		} else if !sftpproxy.IsLoopbackAddr(bindAddr) {
			fmt.Fprintf(os.Stderr, i18n.T("sftp.bind.refuse_global")+"\n", bindAddr)
			fmt.Fprintln(os.Stderr, i18n.T("sftp.bind.refuse_hint"))
			os.Exit(1)
		}
	}

	if err := sftpproxy.ValidateBindAddr(bindAddr, allowRemote); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("sftp.failed")+"\n", err)
		os.Exit(1)
	}

	// If a daemon is already running, ask it to start SFTP proxy.
	// The daemon trusts the AllowRemote decision sent in the mount request.
	if daemonRunning(name) {
		port, fingerprint, daemonID, err := sftpViaDaemon(name, bindAddr, port, allowRemote)
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("sftp.failed")+"\n", err)
			os.Exit(1)
		}
		sftpURL := fmt.Sprintf("sftp://%s:%d", bindAddr, port)
		fmt.Printf(i18n.T("sftp.proxy_started")+"\n", sftpURL)
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

	// No daemon — use the fork-based approach.
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

// SftpStatus shows SFTP proxy status. name empty = all profiles.
func SftpStatus(name string) {
	st := sftpproxy.Status(name)
	if len(st) == 0 {
		if name != "" {
			fmt.Printf(i18n.T("sftp.not_running")+"\n", name)
		} else {
			fmt.Println(i18n.T("sftp.none_running"))
		}
		return
	}
	for n, e := range st {
		line := ""
		if name == "" {
			line = fmt.Sprintf("  %-20s ", n)
		}
		switch e.Status {
		case "ready":
			line += fmt.Sprintf("SFTP proxy: %s (pid %d)", e.URL, e.PID)
		case "starting":
			line += fmt.Sprintf("SFTP proxy: %s (starting)", e.URL)
		default:
			line += fmt.Sprintf("failed: %s", e.Message)
		}
		fmt.Println(line)
	}
}
