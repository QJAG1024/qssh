package sshclient

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"qssh/internal"
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
		Message: "Profile loaded",
	})

	addr := net.JoinHostPort(p.Host, fmt.Sprintf("%d", p.Port))

	// DNS resolve
	progress(internal.StepResult{
		ID: internal.StepDNSResolve, Status: internal.StepRunning,
		Message: fmt.Sprintf("Resolving %s", p.Host),
	})
	resolveStart := time.Now()
	resolvedAddr, err := net.ResolveIPAddr("ip", p.Host)
	if err != nil {
		progress(internal.StepResult{
			ID: internal.StepDNSResolve, Status: internal.StepFailed,
			Message: fmt.Sprintf("DNS resolution failed: %v", err),
			Hint:    "Check the hostname or IP address in the profile",
		})
		return nil, fmt.Errorf("dns resolve: %w", err)
	}
	resolveDone := time.Since(resolveStart)
	progress(internal.StepResult{
		ID: internal.StepDNSResolve, Status: internal.StepDone,
		Detail: fmt.Sprintf("%s → %s (%dms)", p.Host, resolvedAddr, resolveDone.Milliseconds()),
	})

	// Host key callback
	hkCallback, err := hostKeyCallback(p.Host, addr)
	if err != nil {
		return nil, fmt.Errorf("host key callback: %w", err)
	}

	// Auth methods
	authMethods, err := authMethodsForProfile(p)
	if err != nil {
		return nil, fmt.Errorf("auth method: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            p.User,
		Auth:            authMethods,
		HostKeyCallback: hkCallback,
		Timeout:         10 * time.Second,
	}

	// TCP + SSH handshake
	progress(internal.StepResult{
		ID: internal.StepTCPConnect, Status: internal.StepRunning,
		Message: fmt.Sprintf("Connecting to %s", addr),
	})
	connectStart := time.Now()
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		// Determine which step failed based on error type
		if opErr, ok := err.(*net.OpError); ok {
			progress(internal.StepResult{
				ID: internal.StepTCPConnect, Status: internal.StepFailed,
				Message: fmt.Sprintf("TCP connection failed: %s", opErr.Err),
				Hint:    "Confirm the host is online, port is correct, and firewall allows access",
			})
		} else {
			progress(internal.StepResult{
				ID: internal.StepAuthenticate, Status: internal.StepFailed,
				Message: fmt.Sprintf("Authentication failed: %v", err),
				Hint:    "Check credentials in profile: qssh --edit " + p.Name,
			})
		}
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	connectDone := time.Since(connectStart)
	progress(internal.StepResult{
		ID: internal.StepSSHHandshake, Status: internal.StepDone,
		Detail: fmt.Sprintf("Connected in %dms", connectDone.Milliseconds()),
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

	// Request PTY
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "xterm-256color"
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
			Message: fmt.Sprintf("PTY allocation failed: %v", err),
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

	// Start shell
	progress(internal.StepResult{
		ID: internal.StepShellStart, Status: internal.StepRunning,
	})
	if err := sshSesh.Shell(); err != nil {
		progress(internal.StepResult{
			ID: internal.StepShellStart, Status: internal.StepFailed,
			Message: fmt.Sprintf("Shell start failed: %v", err),
		})
		return fmt.Errorf("shell: %w", err)
	}
	progress(internal.StepResult{
		ID: internal.StepShellStart, Status: internal.StepDone,
		Message: "Session established, entering interactive mode",
	})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGWINCH:
				if rawFd >= 0 {
					w, h, _ := term.GetSize(rawFd)
					if w > 0 && h > 0 {
						sshSesh.WindowChange(h, w)
					}
				}
			default:
				// Map common signals to SSH signals
				sshSig := ssh.SIGINT
				switch sig {
				case syscall.SIGTERM:
					sshSig = ssh.SIGTERM
				}
				sshSesh.Signal(sshSig)
			}
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

// Close terminates the SSH connection.
func (s *Session) Close() error {
	if s.sshSession != nil {
		s.sshSession.Close()
	}
	return s.client.Close()
}

// authMethodsForProfile converts a Profile into SSH auth methods.
func authMethodsForProfile(p store.Profile) ([]ssh.AuthMethod, error) {
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

// hostKeyCallback returns an ssh.HostKeyCallback that uses a known_hosts file
// with "accept on first use" semantics (like OpenSSH).
func hostKeyCallback(_, addr string) (ssh.HostKeyCallback, error) {
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