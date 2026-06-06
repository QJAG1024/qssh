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

	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/store"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// Session wraps an SSH connection with PTY management.
type Session struct {
	client    *ssh.Client
	sshSession *ssh.Session
	profile   store.Profile
}

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
		Message: i18n.T("resolving", p.Host),
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
		Detail: i18n.T("dns_resolve.detail", p.Host, resolvedAddr, resolveDone.Milliseconds()),
	})

	// Host key callback
	hkCallback, err := HostKeyCallback(p.Host, addr)
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

	// TCP + SSH handshake
	progress(internal.StepResult{
		ID: internal.StepTCPConnect, Status: internal.StepRunning,
		Message: i18n.T("connecting", addr),
	})
	connectStart := time.Now()
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		// Determine which step failed based on error type
		if opErr, ok := err.(*net.OpError); ok {
			progress(internal.StepResult{
				ID: internal.StepTCPConnect, Status: internal.StepFailed,
				Message: i18n.T("tcp_connect.failed", opErr.Err),
				Hint:    i18n.T("tcp_connect.hint"),
			})
		} else {
			progress(internal.StepResult{
				ID: internal.StepAuthenticate, Status: internal.StepFailed,
				Message: i18n.T("authenticate.failed", err),
				Hint:    i18n.T("authenticate.hint", p.Name),
			})
		}
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
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

	// Attempt raw mode if stdin is a terminal
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

	// Get terminal size
	width, height := 80, 24
	if rawFd >= 0 {
		w, h, err := term.GetSize(rawFd)
		if err == nil && w > 0 && h > 0 {
			width, height = w, h
		}
	}

	// Request PTY — prefer the local TERM, but fall back to a widely
	// available entry so remote systems with a minimal terminfo database
	// (e.g. Debian/PVE) don't complain about a missing one.
	termEnv := os.Getenv("TERM")
	switch termEnv {
	case "xterm", "linux", "vt100":
		// in ncurses-base, available everywhere
	default:
		// xterm-256color is in ncurses-term (not guaranteed),
		// xterm is in ncurses-base and always present.
		termEnv = "xterm"
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	progress(internal.StepResult{
		ID: internal.StepAllocatePTY, Status: internal.StepRunning,
	})
	if err := sshSesh.RequestPty(termEnv, height, width, modes); err != nil {
		progress(internal.StepResult{
			ID: internal.StepAllocatePTY, Status: internal.StepFailed,
			Message: i18n.T("pty_allocate.failed", err),
		})
		return fmt.Errorf("request pty: %w", err)
	}
	progress(internal.StepResult{
		ID: internal.StepAllocatePTY, Status: internal.StepDone,
	})

	// Bridge I/O before starting shell
	sshSesh.Stdin = stdin
	sshSesh.Stdout = stdout
	sshSesh.Stderr = stderr

	// Apply SetEnv options
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
					sshSesh.WindowChange(h, w)
				})
				continue
			}
			// Map common signals to SSH signals
			sshSig := ssh.SIGINT
			if sig == syscall.SIGTERM {
				sshSig = ssh.SIGTERM
			}
			sshSesh.Signal(sshSig)
		}
	}()

	// Wait for session to end
	err = sshSesh.Wait()
	return err
}

// Client returns the underlying SSH client, for use by SFTP etc.
func (s *Session) Client() *ssh.Client {
	return s.client
}

// setSessionEnv applies SetEnv options from the profile to an SSH session.
func setSessionEnv(sshSesh *ssh.Session, p store.Profile) {
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

// DialViaProxy establishes an SSH connection through a jump host.
// proxyClient is an already-connected SSH client to the jump host.
// target is the address of the final host (host:port).
func DialViaProxy(p store.Profile, proxyClient *ssh.Client, targetAddr string, progress internal.ProgressFn) (*Session, error) {
	if progress == nil {
		progress = internal.NopProgress
	}

	progress(internal.StepResult{
		ID: internal.StepDecrypt, Status: internal.StepDone,
		Message: i18n.T("profile.loaded"),
	})

	// Host key callback
	hkCallback, err := HostKeyCallback(p.Host, targetAddr)
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
		Message: i18n.T("proxy.tunneling", proxyClient.RemoteAddr().String(), targetAddr),
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
		Message: i18n.T("proxy.handshake", proxyClient.RemoteAddr().String()),
	})

	client := ssh.NewClient(sshConn, chans, reqs)
	return &Session{client: client, profile: p}, nil
}

// Close terminates the SSH connection.
func (s *Session) Close() error {
	if s.sshSession != nil {
		s.sshSession.Close()
	}
	return s.client.Close()
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

// HostKeyCallback returns an ssh.HostKeyCallback that uses a known_hosts file
// with "accept on first use" semantics (like OpenSSH).
func HostKeyCallback(_, addr string) (ssh.HostKeyCallback, error) {
	khPath := knownHostsFile()
	os.MkdirAll(filepath.Dir(khPath), 0700)

	if _, err := os.Stat(khPath); os.IsNotExist(err) {
		os.WriteFile(khPath, nil, 0600)
	}

	callback, err := knownhosts.New(khPath)
	if err != nil {
		return nil, err
	}

	// Build normalized addresses for writing to known_hosts.
	normalized := []string{knownhosts.Normalize(addr)}
	if hostOnly, _, err := net.SplitHostPort(addr); err == nil && hostOnly != addr {
		normalized = append(normalized, knownhosts.Normalize(hostOnly))
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := callback(hostname, remote, key)
		if err == nil {
			return nil // Key is known and matches
		}
		var keyErr *knownhosts.KeyError
		if !asKeyError(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			// Want contains existing keys — mismatch, possible MITM.
			return fmt.Errorf("host key mismatch for %s: %w", hostname, err)
		}
		// Want is empty — unknown host, accept on first use.
		f, openErr := os.OpenFile(khPath, os.O_APPEND|os.O_WRONLY, 0600)
		if openErr != nil {
			return nil // Accept even if we can't save
		}
		defer f.Close()
		f.WriteString(knownhosts.Line(normalized, key) + "\n")
		return nil
	}, nil
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
		return nil, fmt.Errorf("agent signers: %w", err)
	}
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