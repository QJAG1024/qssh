package sshclient

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/internal/privacy"
	"qssh/store"
)

// Session wraps an SSH connection with PTY management.
type Session struct {
	client     *ssh.Client
	sshSession *ssh.Session
	profile    store.Profile
	// hops holds intermediate jump-host clients that must stay open for the
	// final tunnel to work. Closed (outermost first) after the final client.
	hops []*ssh.Client
}

// ProfileLookup resolves a profile by name (used for jump-host chains).
type ProfileLookup func(name string) (store.Profile, bool)

// Dial establishes an SSH connection using the given profile.
// Reports progress via the callback.
func Dial(p store.Profile, progress internal.ProgressFn) (*Session, error) {
	if progress == nil {
		progress = internal.NopProgress
	}

	progress(internal.StepResult{
		ID: internal.StepDecrypt, Status: internal.StepDone,
		Message: i18n.T("profile.loaded"),
	})

	addr := net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port))

	// DNS resolve
	progress(internal.StepResult{
		ID: internal.StepDNSResolve, Status: internal.StepRunning,
		Message: i18n.T("resolving", privacy.Host(p.Host)),
	})
	resolveStart := time.Now()
	resolvedAddr, err := net.ResolveIPAddr("ip", p.Host)
	if err != nil {
		progress(internal.StepResult{
			ID: internal.StepDNSResolve, Status: internal.StepFailed,
			Message: i18n.T("dns_resolve.failed", err),
			Hint:    i18n.T("dns_resolve.hint"),
		})
		return nil, fmt.Errorf("dns resolve: %w", err)
	}
	resolveDone := time.Since(resolveStart)
	progress(internal.StepResult{
		ID: internal.StepDNSResolve, Status: internal.StepDone,
		Detail: i18n.T("dns_resolve.detail", privacy.Host(p.Host), privacy.Host(resolvedAddr.String()), resolveDone.Milliseconds()),
	})

	// Host key callback
	hkCallback, err := HostKeyCallback(p.Options, addr)
	if err != nil {
		return nil, fmt.Errorf("host key callback: %w", err)
	}

	// Auth methods
	authMethods, err := AuthMethodsForProfile(p)
	if err != nil {
		return nil, fmt.Errorf("auth method: %w", err)
	}

	timeout := 10 * time.Second
	if v, ok := p.Options["ConnectTimeout"]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}

	config := &ssh.ClientConfig{
		User:            p.User,
		Auth:            authMethods,
		HostKeyCallback: hkCallback,
		Timeout:         timeout,
	}

	// TCP + SSH handshake with a hard deadline covering authentication.
	// ssh.ClientConfig.Timeout only bounds the TCP dial; the handshake and
	// password/keyboard-interactive exchange can otherwise hang forever on an
	// unresponsive peer. We dial manually and SetDeadline around NewClientConn.
	progress(internal.StepResult{
		ID: internal.StepTCPConnect, Status: internal.StepRunning,
		Message: i18n.T("connecting", privacy.Addr(addr)),
	})
	connectStart := time.Now()

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		progress(internal.StepResult{
			ID: internal.StepTCPConnect, Status: internal.StepFailed,
			Message: i18n.T("tcp_connect.failed", err),
			Hint:    i18n.T("tcp_connect.hint"),
		})
		return nil, fmt.Errorf("tcp dial: %w", err)
	}
	// Cover the whole handshake (key exchange + auth) with the timeout; clear
	// it once the connection is established so long-running sessions are not
	// cut off by the connect deadline.
	deadline := time.Now().Add(timeout)
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetDeadline(deadline)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetDeadline(time.Time{})
	}
	if err != nil {
		conn.Close()
		progress(internal.StepResult{
			ID: internal.StepAuthenticate, Status: internal.StepFailed,
			Message: i18n.T("authenticate.failed", err),
			Hint:    i18n.T("authenticate.hint", p.Name),
		})
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	connectDone := time.Since(connectStart)
	progress(internal.StepResult{
		ID: internal.StepSSHHandshake, Status: internal.StepDone,
		Detail: i18n.T("connected", connectDone.Milliseconds()),
	})

	return &Session{client: client, profile: p}, nil
}

// InteractiveShell opens an interactive shell with PTY, signal forwarding,
// and window resize handling. Blocks until the session exits.
// If stdin is a terminal, it is switched to raw mode for proper handling.
func (s *Session) InteractiveShell(stdin io.Reader, stdout, stderr io.Writer, progress internal.ProgressFn) error {
	if progress == nil {
		progress = internal.NopProgress
	}

	sshSesh, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	s.sshSession = sshSesh
	defer sshSesh.Close()

	// Attempt raw mode if stdin is a terminal.
	// Local raw mode is client-side (keystrokes); remote PTY modes are separate.
	var rawFd int = -1
	var oldState *term.State
	if f, ok := stdin.(*os.File); ok {
		rawFd = int(f.Fd())
		if term.IsTerminal(rawFd) {
			oldState, err = term.MakeRaw(rawFd)
			if err != nil {
				return fmt.Errorf("make raw terminal: %w", err)
			}
			defer term.Restore(rawFd, oldState)
		}
	}

	width, height := terminalSize(rawFd)

	// TERM: passthrough local value by default so TUI apps (docker compose, etc.)
	// see the same capabilities as a native OpenSSH session. Escape hatch:
	//   qssh --config set term.mode compat        (global)
	//   qssh --edit <p> --set-option term.mode=compat   (per-profile)
	// forces a minimal terminfo entry for hosts without ncurses-term.
	termEnv := resolveTermEnv(s.profile.Options)
	modes := defaultPTYModes()

	progress(internal.StepResult{
		ID: internal.StepAllocatePTY, Status: internal.StepRunning,
	})
	if err := sshSesh.RequestPty(termEnv, height, width, modes); err != nil {
		// If a fancy TERM is unknown on the server, retry once with xterm.
		if termEnv != "xterm" {
			if err2 := sshSesh.RequestPty("xterm", height, width, modes); err2 == nil {
				termEnv = "xterm"
				err = nil
			}
		}
		if err != nil {
			progress(internal.StepResult{
				ID: internal.StepAllocatePTY, Status: internal.StepFailed,
				Message: i18n.T("pty_allocate.failed", err),
			})
			return fmt.Errorf("request pty: %w", err)
		}
	}
	// Re-assert winsize after pty-req (some servers ignore initial size).
	_ = sshSesh.WindowChange(height, width)
	progress(internal.StepResult{
		ID: internal.StepAllocatePTY, Status: internal.StepDone,
	})

	// Bridge I/O before starting shell
	sshSesh.Stdin = stdin
	sshSesh.Stdout = stdout
	sshSesh.Stderr = stderr

	// Best-effort env: keep TERM/COLORTERM aligned even if pty-req name differs.
	_ = sshSesh.Setenv("TERM", termEnv)
	if color := os.Getenv("COLORTERM"); color != "" {
		_ = sshSesh.Setenv("COLORTERM", color)
	}

	// Apply profile SetEnv options
	setSessionEnv(sshSesh, s.profile)

	// Start shell
	progress(internal.StepResult{
		ID: internal.StepShellStart, Status: internal.StepRunning,
	})
	if err := sshSesh.Shell(); err != nil {
		progress(internal.StepResult{
			ID: internal.StepShellStart, Status: internal.StepFailed,
			Message: i18n.T("shell_start.failed", err),
		})
		return fmt.Errorf("shell: %w", err)
	}
	// Once more after shell start — helps TUIs that query size on first paint.
	_ = sshSesh.WindowChange(height, width)
	progress(internal.StepResult{
		ID: internal.StepShellStart, Status: internal.StepDone,
		Message: i18n.T("session.ready"),
	})
	sigCh := make(chan os.Signal, 1)
	winchSignals := windowChangeSignals()
	notifySignals := make([]os.Signal, 0, 2+len(winchSignals))
	notifySignals = append(notifySignals, syscall.SIGINT, syscall.SIGTERM)
	for _, ws := range winchSignals {
		notifySignals = append(notifySignals, ws)
	}
	signal.Notify(sigCh, notifySignals...)
	defer signal.Stop(sigCh)

	go func() {
		for sig := range sigCh {
			isWinch := false
			for _, ws := range winchSignals {
				if sig == ws {
					isWinch = true
					break
				}
			}
			if isWinch {
				onWindowChange(rawFd, func(h, w int) {
					if h > 0 && w > 0 {
						_ = sshSesh.WindowChange(h, w)
					}
				})
				continue
			}
			// Map common signals to SSH signals
			sshSig := ssh.SIGINT
			if sig == syscall.SIGTERM {
				sshSig = ssh.SIGTERM
			}
			_ = sshSesh.Signal(sshSig)
		}
	}()

	// Wait for session to end
	err = sshSesh.Wait()
	return err
}

// resolveTermEnv picks the PTY $TERM string for RequestPty.
// Default is passthrough of the local TERM (OpenSSH-like).
// term.mode=compat forces a widely-available entry for broken remote terminfo.
// Per-profile override: profile Options["term.mode"] wins over the global key.
// Terminal types unlikely to exist on remote hosts are mapped to xterm-256color.
func resolveTermEnv(profileOpts map[string]string) string {
	mode := strings.ToLower(strings.TrimSpace(internal.EffectiveOption(profileOpts, "term.mode")))
	local := strings.TrimSpace(os.Getenv("TERM"))
	if mode == "compat" {
		// Minimal set present in ncurses-base on almost every distro.
		switch local {
		case "xterm", "linux", "vt100":
			return local
		default:
			return "xterm"
		}
	}
	if local == "" {
		return "xterm-256color"
	}
	// Passthrough with fallback: terminal types that are unlikely to have
	// a remote terminfo entry (ghostty, kitty, wezterm, etc.) → xterm-256color.
	if isUnlikelyRemoteTerm(local) {
		return "xterm-256color"
	}
	return local
}

// isUnlikelyRemoteTerm returns true for terminal types that are typically
// only installed on the local machine and not on remote hosts.
func isUnlikelyRemoteTerm(term string) bool {
	switch strings.ToLower(term) {
	case "xterm-ghostty", "xterm-kitty", "kitty",
		"wezterm", "xterm-wezterm",
		"alacritty",
		"contour",
		"foot",
		"rio":
		return true
	}
	return false
}

// terminalSize returns a sane PTY size. Never returns 0x0 (breaks TUIs).
func terminalSize(rawFd int) (width, height int) {
	width, height = 80, 24
	if rawFd >= 0 {
		if w, h, err := term.GetSize(rawFd); err == nil && w > 0 && h > 0 {
			return w, h
		}
	}
	return width, height
}

// defaultPTYModes is a compact OpenSSH-like mode set for interactive shells.
// Local MakeRaw remains client-side; these apply to the remote PTY.
func defaultPTYModes() ssh.TerminalModes {
	return ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.ECHOE:         1,
		ssh.ECHOK:         1,
		ssh.ECHONL:        0,
		ssh.ICANON:        1,
		ssh.ISIG:          1,
		ssh.ICRNL:         1,
		ssh.OPOST:         1,
		ssh.ONLCR:         1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
}

// Client returns the underlying SSH client, for use by SFTP etc.
func (s *Session) Client() *ssh.Client {
	return s.client
}

// ApplySessionEnv applies SetEnv options from the profile to an SSH session.
// Supports comma-separated KEY=VALUE pairs in Options["SetEnv"].
func ApplySessionEnv(sshSesh *ssh.Session, p store.Profile) {
	if env, ok := p.Options["SetEnv"]; ok && env != "" {
		// Support comma-separated KEY=VALUE pairs
		for _, pair := range strings.Split(env, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key, val := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			if key != "" {
				_ = sshSesh.Setenv(key, val) // best effort; server may reject
			}
		}
	}
}

// setSessionEnv is kept as an alias for call sites inside this package.
func setSessionEnv(sshSesh *ssh.Session, p store.Profile) {
	ApplySessionEnv(sshSesh, p)
}

// DialViaProxy establishes an SSH connection through a jump host.
// proxyClient is an already-connected SSH client to the jump host.
// target is the address of the final host (host:port).
// hops are intermediate clients that must remain open; ownership transfers to
// the returned Session and they are closed with it.
func DialViaProxy(p store.Profile, proxyClient *ssh.Client, targetAddr string, progress internal.ProgressFn, hops ...*ssh.Client) (*Session, error) {
	if progress == nil {
		progress = internal.NopProgress
	}

	progress(internal.StepResult{
		ID: internal.StepDecrypt, Status: internal.StepDone,
		Message: i18n.T("profile.loaded"),
	})

	// Host key callback
	hkCallback, err := HostKeyCallback(p.Options, targetAddr)
	if err != nil {
		return nil, fmt.Errorf("host key callback: %w", err)
	}

	authMethods, err := AuthMethodsForProfile(p)
	if err != nil {
		return nil, fmt.Errorf("auth method: %w", err)
	}

	timeout := 10 * time.Second
	if v, ok := p.Options["ConnectTimeout"]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}

	config := &ssh.ClientConfig{
		User:            p.User,
		Auth:            authMethods,
		HostKeyCallback: hkCallback,
		Timeout:         timeout,
	}

	progress(internal.StepResult{
		ID: internal.StepProxyConnect, Status: internal.StepRunning,
		Message: i18n.T("proxy.tunneling", privacy.Addr(proxyClient.RemoteAddr().String()), privacy.Addr(targetAddr)),
	})

	proxyConn, err := proxyClient.Dial("tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("proxy tunnel: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(proxyConn, targetAddr, config)
	if err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("target handshake via proxy: %w", err)
	}

	progress(internal.StepResult{
		ID: internal.StepProxyConnect, Status: internal.StepDone,
		Message: i18n.T("proxy.handshake", privacy.Addr(proxyClient.RemoteAddr().String())),
	})

	client := ssh.NewClient(sshConn, chans, reqs)
	return &Session{client: client, profile: p, hops: hops}, nil
}

// DialProfile dials a profile, following any Proxy jump-host chain via lookup.
// This is the single entry point for connect, daemon, and SFTP paths.
func DialProfile(p store.Profile, lookup ProfileLookup, progress internal.ProgressFn) (*Session, error) {
	if progress == nil {
		progress = internal.NopProgress
	}
	if p.Proxy == "" {
		return Dial(p, progress)
	}
	if lookup == nil {
		return nil, fmt.Errorf("profile %q requires proxy %q but no profile lookup provided", p.Name, p.Proxy)
	}
	return dialViaProxyChain(p, lookup, progress)
}

// dialViaProxyChain walks the proxy chain and tunnels through each hop.
// chain order is [innermostProxy, ..., outermostProxy].
func dialViaProxyChain(p store.Profile, lookup ProfileLookup, progress internal.ProgressFn) (*Session, error) {
	chain, err := buildProxyChain(p, lookup)
	if err != nil {
		return nil, err
	}

	// Dial the outermost proxy directly.
	last := chain[len(chain)-1]
	progress(internal.StepResult{
		ID: internal.StepProxyConnect, Status: internal.StepRunning,
		Message: i18n.T("proxy.connecting", last.Name),
	})
	outer, err := Dial(last, progress)
	if err != nil {
		return nil, fmt.Errorf("proxy %s: %w", last.Name, err)
	}

	// hops[0] is outermost; keep every hop client alive until final Close.
	hops := []*ssh.Client{outer.Client()}
	proxyClient := outer.Client()

	// Walk inner proxies, tunneling through each.
	for i := len(chain) - 2; i >= 0; i-- {
		target := chain[i]
		addr := net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port))
		progress(internal.StepResult{
			ID: internal.StepProxyConnect, Status: internal.StepRunning,
			Message: i18n.T("proxy.tunneling", privacy.Addr(proxyClient.RemoteAddr().String()), privacy.Addr(addr)),
		})
		tunnel, err := proxyClient.Dial("tcp", addr)
		if err != nil {
			closeHops(hops)
			return nil, fmt.Errorf("proxy tunnel to %s: %w", target.Name, err)
		}
		sshConn, chans, reqs, err := newClientConn(tunnel, addr, target)
		if err != nil {
			tunnel.Close()
			closeHops(hops)
			return nil, fmt.Errorf("proxy handshake %s: %w", target.Name, err)
		}
		proxyClient = ssh.NewClient(sshConn, chans, reqs)
		hops = append(hops, proxyClient)
	}

	// Tunnel from innermost proxy to final target.
	targetAddr := net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port))
	session, err := DialViaProxy(p, proxyClient, targetAddr, progress, hops...)
	if err != nil {
		closeHops(hops)
		return nil, err
	}
	return session, nil
}

// buildProxyChain resolves the proxy chain from innermost to outermost.
func buildProxyChain(p store.Profile, lookup ProfileLookup) ([]store.Profile, error) {
	seen := map[string]bool{p.Name: true}
	chain := make([]store.Profile, 0)
	cur := p
	for cur.Proxy != "" {
		if seen[cur.Proxy] {
			return nil, fmt.Errorf("proxy cycle detected: %s -> %s", cur.Name, cur.Proxy)
		}
		seen[cur.Proxy] = true
		pp, exists := lookup(cur.Proxy)
		if !exists {
			return nil, fmt.Errorf("proxy profile %q not found", cur.Proxy)
		}
		chain = append(chain, pp)
		cur = pp
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("empty proxy chain for %q", p.Name)
	}
	return chain, nil
}

// newClientConn performs an SSH handshake over an existing net.Conn.
func newClientConn(c net.Conn, addr string, p store.Profile) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	hkCallback, err := HostKeyCallback(p.Options, addr)
	if err != nil {
		return nil, nil, nil, err
	}
	authMethods, err := AuthMethodsForProfile(p)
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

func closeHops(hops []*ssh.Client) {
	// Close from innermost tunnel client back to outermost dial.
	for i := len(hops) - 1; i >= 0; i-- {
		if hops[i] != nil {
			hops[i].Close()
		}
	}
}

// Close terminates the SSH connection and any jump-host hops.
func (s *Session) Close() error {
	if s.sshSession != nil {
		s.sshSession.Close()
	}
	var err error
	if s.client != nil {
		err = s.client.Close()
	}
	closeHops(s.hops)
	return err
}

// AuthMethodsForProfile converts a Profile into SSH auth methods.
func AuthMethodsForProfile(p store.Profile) ([]ssh.AuthMethod, error) {
	switch p.Auth {
	case store.AuthPassword:
		return []ssh.AuthMethod{ssh.Password(p.Password)}, nil

	case store.AuthKey:
		key, err := os.ReadFile(expandPath(p.KeyPath))
		if err != nil {
			return nil, fmt.Errorf("read key file %s: %w", p.KeyPath, err)
		}
		if p.KeyPassphrase != "" {
			signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(p.KeyPassphrase))
			if err != nil {
				return nil, fmt.Errorf("parse encrypted key: %w", err)
			}
			return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil

	case store.AuthAgent:
		agent, err := agentAuth()
		if err != nil {
			return nil, fmt.Errorf("ssh agent: %w", err)
		}
		return []ssh.AuthMethod{agent}, nil

	case store.AuthKeyboardInteractive:
		return []ssh.AuthMethod{ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			if len(questions) == 0 {
				return nil, nil
			}
			answers := make([]string, len(questions))
			for i, q := range questions {
				if echos[i] {
					fmt.Printf("%s: ", q)
					fmt.Scanf("%s", &answers[i])
				} else {
					pass, err := internal.ReadPassword(q)
					if err != nil {
						return nil, err
					}
					answers[i] = pass
				}
			}
			return answers, nil
		})}, nil

	default:
		return nil, fmt.Errorf("unsupported auth method: %s", p.Auth)
	}
}

// hostKeyMode returns the configured host-key policy: "tofu" (default) or "strict".
// Config key: hostkey.mode. Per-profile override: profile Options["hostkey.mode"]
// wins over the global key (empty profile value falls through to global).
// Unknown or empty values default to tofu; only the explicit value "strict"
// enables strict mode. Malformed values that look like an attempt at a security
// mode (anything other than tofu/strict/empty) are treated as a hard error so a
// typo cannot silently weaken policy.
// A corrupt config file also fails closed (cannot silently drop strict→tofu).
func hostKeyMode(profileOpts map[string]string) (string, error) {
	cfg := internal.OpenConfig(internal.DefaultConfigPath())
	if cfg != nil && cfg.LoadError() != nil {
		return "", fmt.Errorf("hostkey.mode unavailable: %w", cfg.LoadError())
	}
	mode := strings.ToLower(strings.TrimSpace(internal.EffectiveOption(profileOpts, "hostkey.mode")))
	switch mode {
	case "", "tofu":
		return "tofu", nil
	case "strict":
		return "strict", nil
	default:
		return "", fmt.Errorf("invalid hostkey.mode %q (supported: tofu, strict)", mode)
	}
}

// HostKeyCallback returns an ssh.HostKeyCallback that uses a known_hosts file.
// Default policy is TOFU (accept on first use) with fingerprint logged to stderr.
// Set hostkey.mode=strict in config to reject unknown hosts.
// TOFU persistence is fail-closed: lock, re-check, write, fsync, and close must
// all succeed or the connection is aborted. Host identity is port-scoped
// ([host]:port) so distinct SSH ports do not share a key entry.
func HostKeyCallback(profileOpts map[string]string, addr string) (ssh.HostKeyCallback, error) {
	khPath := knownHostsFile()
	if err := os.MkdirAll(filepath.Dir(khPath), 0700); err != nil {
		return nil, fmt.Errorf("known_hosts dir: %w", err)
	}

	// Port-scoped identity only — do NOT also write bare hostname entries,
	// which would let host:22 and host:2222 share the same TOFU identity.
	normalized := []string{knownhosts.Normalize(addr)}

	mode, modeErr := hostKeyMode(profileOpts)
	if modeErr != nil {
		return nil, modeErr
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// Hold a cross-process lock for the full check+append so two concurrent
		// first-use connections cannot each accept a different key.
		fl, err := internal.Lock(khPath)
		if err != nil {
			return fmt.Errorf("lock known_hosts: %w", err)
		}
		defer fl.Unlock()

		// Ensure the file exists under the lock; use O_CREATE without O_TRUNC
		// so a concurrent first-use writer cannot be overwritten with an
		// empty file. If another process created it, we just open it.
		if _, err := os.Stat(khPath); os.IsNotExist(err) {
			f, err := os.OpenFile(khPath, os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("create known_hosts: %w", err)
			}
			_ = f.Close()
		}

		// Re-load under the lock so concurrent first-use writers are visible.
		callback, err := knownhosts.New(khPath)
		if err != nil {
			return fmt.Errorf("reload known_hosts: %w", err)
		}
		err = callback(hostname, remote, key)
		if err == nil {
			return nil // Key is known and matches
		}
		var keyErr *knownhosts.KeyError
		if !asKeyError(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			// Want contains existing keys — mismatch, possible MITM.
			return fmt.Errorf("host key mismatch for %s: %w", privacy.Host(hostname), err)
		}
		// Want is empty — unknown host.
		fp := ssh.FingerprintSHA256(key)
		displayHost := privacy.Host(hostname)
		if mode == "strict" {
			return fmt.Errorf("unknown host key for %s (%s); hostkey.mode=strict rejects first-use", displayHost, fp)
		}
		// TOFU: log fingerprint for audit, then accept and persist.
		// stderr is visible for interactive connect; daemon forks discard stderr,
		// so also append to hostkey.log under the config dir.
		// Host is redacted when privacy mode is on (display + audit log).
		msg := fmt.Sprintf("host key accepted (TOFU): %s %s %s", displayHost, key.Type(), fp)
		fmt.Fprintln(os.Stderr, msg)
		if logPath := hostKeyAuditLog(); logPath != "" {
			if lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
				fmt.Fprintf(lf, "%s %s\n", time.Now().Format(time.RFC3339), msg)
				lf.Close()
			}
		}
		f, openErr := os.OpenFile(khPath, os.O_APPEND|os.O_WRONLY, 0600)
		if openErr != nil {
			return fmt.Errorf("cannot persist host key to %s: %w", khPath, openErr)
		}
		if _, werr := f.WriteString(knownhosts.Line(normalized, key) + "\n"); werr != nil {
			f.Close()
			return fmt.Errorf("cannot write host key to %s: %w", khPath, werr)
		}
		if serr := f.Sync(); serr != nil {
			f.Close()
			return fmt.Errorf("cannot sync host key to %s: %w", khPath, serr)
		}
		if cerr := f.Close(); cerr != nil {
			return fmt.Errorf("cannot close known_hosts %s: %w", khPath, cerr)
		}
		return nil
	}, nil
}

func hostKeyAuditLog() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "qssh", "hostkey.log")
}

// asKeyError is a helper to type-assert *knownhosts.KeyError.
func asKeyError(err error, target **knownhosts.KeyError) bool {
	*target, _ = err.(*knownhosts.KeyError)
	return *target != nil
}

// knownHostsFile returns the path to the QSSH known_hosts file.
// Override with QSSH_KNOWN_HOSTS env var (used in tests).
func knownHostsFile() string {
	if p := os.Getenv("QSSH_KNOWN_HOSTS"); p != "" {
		return p
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "known_hosts")
}

// expandPath expands ~ to the home directory.
func expandPath(path string) string {
	if len(path) > 1 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return home + path[1:]
		}
	}
	return path
}

// agentAuth attempts to use the SSH agent for authentication.
// The agent unix connection is kept open for the lifetime of the returned
// AuthMethod (needed for signing); callers do not Close it explicitly.
// Signers are fetched once so a missing agent fails fast.
func agentAuth() (ssh.AuthMethod, error) {
	agentSock := os.Getenv("SSH_AUTH_SOCK")
	if agentSock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}
	conn, err := net.Dial("unix", agentSock)
	if err != nil {
		return nil, fmt.Errorf("dial agent: %w", err)
	}
	agentClient := agent.NewClient(conn)
	signers, err := agentClient.Signers()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("agent signers: %w", err)
	}
	if len(signers) == 0 {
		conn.Close()
		return nil, fmt.Errorf("ssh agent has no keys")
	}
	// PublicKeys keeps signers; agent conn must stay open for subsequent sign ops.
	// golang.org/x/crypto/ssh copies signers into the method; agent.NewClient
	// still uses conn for Sign. Attach a finalizer-friendly wrapper by
	// returning PublicKeys of already-fetched signers (local private material
	// from agent is not exported — Signers() returns agent-backed signers that
	// need the conn). Keep conn open intentionally.
	return ssh.PublicKeys(signers...), nil
}

// RunCommand executes a single command on the remote host.
// It connects stdout/stderr to the local process and optionally stdin for
// interactive commands. Returns the remote exit code.
func (s *Session) RunCommand(cmd string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	sshSesh, err := s.client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("new session: %w", err)
	}
	defer sshSesh.Close()

	sshSesh.Stdin = stdin
	sshSesh.Stdout = stdout
	sshSesh.Stderr = stderr

	setSessionEnv(sshSesh, s.profile)

	if err := sshSesh.Run(cmd); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		return -1, err
	}
	return 0, nil
}
