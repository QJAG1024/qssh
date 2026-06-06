package cmd

import (
	"fmt"
	"net"
	"os"
	"time"

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/sshclient"
	"qssh/store"
	"golang.org/x/crypto/ssh"
)

// Connect establishes an SSH connection to the named profile.
func Connect(name string) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("store.open_error"), err)
		os.Exit(1)
	}

	p, exists := s.Get(name)
	if !exists {
		fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", name)
		os.Exit(1)
	}

	internal.RenderProfileHeader(p.Name, p.User, p.Host, p.Port)

	session, err := dialProfile(p, s)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("connect.failed"))
		os.Exit(1)
	}
	defer session.Close()

	startTime := time.Now()
	if err := session.InteractiveShell(os.Stdin, os.Stdout, os.Stderr, internal.RenderProgress); err != nil {
		fmt.Fprintf(os.Stderr, "\n"+i18n.T("connect.ended")+"\n", err)
	}

	duration := time.Since(startTime)
	internal.RenderSummary(p.Name, formatDuration(duration))

	s.Touch(name)
	internal.AppendHistory(&internal.HistoryEntry{
		Profile:  p.Name,
		Duration: formatDuration(duration),
		Command:  "",
		ExitCode: 0,
	})
}

// dialProfile connects to a profile, resolving any proxy chain.
func dialProfile(p store.Profile, st *store.Store) (*sshclient.Session, error) {
	if p.Proxy == "" {
		return sshclient.Dial(p, internal.RenderProgress)
	}
	return dialViaProxyChain(p, st)
}

// dialViaProxyChain walks the proxy chain and tunnels through each hop.
func dialViaProxyChain(p store.Profile, st *store.Store) (*sshclient.Session, error) {
	buildProxyChain(p, st)
	chain := buildProxyChain(p, st)

	// Dial the outermost proxy directly.
	last := chain[len(chain)-1]
	internal.RenderProgress(internal.StepResult{
		ID: internal.StepProxyConnect, Status: internal.StepRunning,
		Message: i18n.T("proxy.connecting", last.Name),
	})
	ps, err := sshclient.Dial(last, internal.RenderProgress)
	if err != nil {
		return nil, fmt.Errorf("proxy %s: %w", last.Name, err)
	}
	proxyClient := ps.Client()

	// Walk inner proxies, tunneling through each.
	// chain = [innermostProxy, ..., outermostProxy]
	// outermostProxy (chain[n-1]) is already dialed.
	for i := len(chain) - 2; i >= 0; i-- {
		target := chain[i]
		addr := net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port))
		internal.RenderProgress(internal.StepResult{
			ID: internal.StepProxyConnect, Status: internal.StepRunning,
			Message: i18n.T("proxy.tunneling", proxyClient.RemoteAddr().String(), addr),
		})
		tunnel, err := proxyClient.Dial("tcp", addr)
		if err != nil {
			ps.Close()
			return nil, fmt.Errorf("proxy tunnel to %s: %w", target.Name, err)
		}
		sshConn, chans, reqs, err := newClientConn(tunnel, addr, target)
		if err != nil {
			tunnel.Close()
			ps.Close()
			return nil, fmt.Errorf("proxy handshake %s: %w", target.Name, err)
		}
		proxyClient = ssh.NewClient(sshConn, chans, reqs)
	}

	// Tunnel from innermost proxy to final target.
	targetAddr := net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port))
	return sshclient.DialViaProxy(p, proxyClient, targetAddr, internal.RenderProgress)
}

// buildProxyChain resolves the proxy chain from innermost to outermost.
func buildProxyChain(p store.Profile, st *store.Store) []store.Profile {
	seen := map[string]bool{p.Name: true}
	chain := make([]store.Profile, 0)
	cur := p
	for cur.Proxy != "" {
		if seen[cur.Proxy] {
			fmt.Fprintf(os.Stderr, "proxy cycle detected: %s -> %s\n", cur.Name, cur.Proxy)
			os.Exit(1)
		}
		seen[cur.Proxy] = true
		pp, exists := st.Get(cur.Proxy)
		if !exists {
			fmt.Fprintf(os.Stderr, i18n.T("profile.not_found")+"\n", cur.Proxy)
			os.Exit(1)
		}
		chain = append(chain, pp)
		cur = pp
	}
	return chain
}

// newClientConn performs an SSH handshake over an existing net.Conn.
func newClientConn(c net.Conn, addr string, p store.Profile) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	hkCallback, err := sshclient.HostKeyCallback(p.Host, addr)
	if err != nil {
		return nil, nil, nil, err
	}
	authMethods, err := sshclient.AuthMethodsForProfile(p)
	if err != nil {
		return nil, nil, nil, err
	}
	config := &ssh.ClientConfig{
		User:            p.User,
		Auth:            authMethods,
		HostKeyCallback: hkCallback,
	}
	return ssh.NewClientConn(c, addr, config)
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}