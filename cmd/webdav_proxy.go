package cmd

import (
	"fmt"
	"os"
	"strings"

	"qssh/internal/i18n"
	"qssh/webdav"
)

func init() {
	// Wire up store opener for the WebDAV daemon.
	webdav.SetOpenStore(openStore)
}

// WebdavStart starts a WebDAV server for the profile.
// bindAddr defaults to loopback; non-loopback requires allowRemote.
func WebdavStart(name, bindAddr string, port int, allowRemote bool) {
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	if allowRemote {
		fmt.Fprintln(os.Stderr, i18n.T("webdav.warn_remote"))
	}
	url, err := webdav.Start(name, bindAddr, port, allowRemote)
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
func WebdavDaemon(name, port, bindAddr string, allowRemote bool) {
	webdav.Daemon(name, port, bindAddr, allowRemote)
}
