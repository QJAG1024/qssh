package cmd

import (
	"fmt"
	"os"
	"strings"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sftpproxy"
	"qssh/webdav"
)

func init() {
	// Wire up store opener for the WebDAV daemon.
	webdav.SetOpenStore(openStore)
}

// WebdavStart starts a WebDAV server for the profile.
// Effective bind address: CLI --bind > profile webdav.bind > global
// webdav.bind > 127.0.0.1. Token mode: profile/global webdav.token_mode
// (auto|always), default auto.
func WebdavStart(name, cliBind string, port int, allowRemote bool, readonly bool) {
	// Load the profile for per-profile webdav.bind / webdav.token_mode.
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

	bindAddr := cliBind
	if bindAddr == "" {
		bindAddr = internal.EffectiveOption(profileOpts, "webdav.bind")
	}
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}

	tokenMode := internal.EffectiveOption(profileOpts, "webdav.token_mode")
	if tokenMode == "" {
		tokenMode = "auto"
	}

	if allowRemote || !isLoopback(bindAddr) {
		fmt.Fprintln(os.Stderr, i18n.T("webdav.warn_remote"))
	}

	url, err := webdav.Start(name, bindAddr, port, allowRemote, tokenMode, readonly)
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("webdav.failed")+"\n", err)
		os.Exit(1)
	}
	// Print both protocol forms: dav:// (gio/davfs2/Linux, KDE) and http://
	// (Finder "Connect to Server", Windows Map Network Drive).
	fmt.Println(i18n.T("webdav.url"))
	if strings.Contains(url, "?token=") {
		// Token-auth URL: keep the token in the query for HTTP clients; for
		// dav:// clients the token goes in the URL as user:token@.
		base := strings.SplitN(url, "?", 2)[0]
		token := strings.TrimPrefix(strings.SplitN(url, "?", 2)[1], "token=")
		hostport := strings.TrimSuffix(strings.TrimPrefix(base, "http://"), "/")
		fmt.Printf("  dav://qssh:%s@%s/\n", token, hostport)
		fmt.Printf("  %s\n", url)
		fmt.Fprintln(os.Stderr, i18n.T("webdav.token_hint"))
	} else {
		fmt.Printf("  dav://%s\n", strings.TrimPrefix(url, "http://"))
		fmt.Printf("  %s\n", url)
	}
	fmt.Fprintln(os.Stderr, i18n.T("webdav.mount_hint"))
	if readonly {
		fmt.Fprintln(os.Stderr, i18n.T("webdav.readonly_hint"))
	}
}

// isLoopback reports whether a bind address resolves only to loopback.
func isLoopback(addr string) bool {
	return sftpproxy.IsLoopbackAddr(addr)
}

// WebdavStop stops the WebDAV server for the profile.
func WebdavStop(name string) {
	if err := webdav.Stop(name); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("webdav.stop_failed")+"\n", err)
		os.Exit(1)
	}
	fmt.Println(i18n.T("webdav.stopped"))
}

// WebdavDaemon is the hidden entry point for the WebDAV worker.
func WebdavDaemon(name, port, bindAddr string, allowRemote bool, tokenMode string, readonly bool) {
	webdav.Daemon(name, port, bindAddr, allowRemote, tokenMode, readonly)
}

// WebdavStatus shows WebDAV mount status. name empty = all profiles.
func WebdavStatus(name string) {
	st := webdav.Status(name)
	if len(st) == 0 {
		if name != "" {
			fmt.Printf(i18n.T("webdav.not_running")+"\n", name)
		} else {
			fmt.Println(i18n.T("webdav.none_running"))
		}
		return
	}
	fmt.Println(i18n.T("webdav.url"))
	for n, e := range st {
		if name == "" {
			fmt.Printf("  %-20s ", n)
		}
		switch e.Status {
		case "ready":
			fmt.Printf("%s (pid %d)\n", e.URL, e.PID)
		case "starting":
			fmt.Printf("%s (starting)\n", e.URL)
		default:
			fmt.Printf("failed: %s\n", e.Message)
		}
	}
}
