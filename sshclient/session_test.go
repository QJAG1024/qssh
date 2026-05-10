package sshclient

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"qssh/internal"
	"qssh/store"
	"golang.org/x/crypto/ssh"
)

// testHostKey is a static RSA key for the test SSH server.
var testHostKey ssh.Signer

func init() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("generate test host key: %v", err))
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		panic(fmt.Sprintf("create signer: %v", err))
	}
	testHostKey = signer
}

// startTestSSHServer starts a minimal SSH server for testing.
// Returns the address and password callback.
func startTestSSHServer(t *testing.T, passwordCallback func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error)) (string, *ssh.ServerConfig) {
	t.Helper()

	config := &ssh.ServerConfig{
		PasswordCallback: passwordCallback,
	}
	config.AddHostKey(testHostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(nConn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						newChannel.Reject(ssh.UnknownChannelType, "unknown")
						continue
					}
					ch, reqs, err := newChannel.Accept()
					if err != nil {
						continue
					}
					go handleSessionReqs(reqs, ch)
					// Write a simple prompt and wait
					ch.Write([]byte("Welcome to test SSH server\r\n"))
					// Read and echo back until exit
					buf := make([]byte, 1024)
					for {
						n, err := ch.Read(buf)
						if err != nil {
							break
						}
						ch.Write(buf[:n])
					}
					ch.Close()
				}
			}()
		}
	}()

	return listener.Addr().String(), config
}

// handleSessionReqs replies to PTY and shell requests from the client.
// Must run in a goroutine.
func handleSessionReqs(reqs <-chan *ssh.Request, ch ssh.Channel) {
	for req := range reqs {
		switch req.Type {
		case "shell", "pty-req":
			req.Reply(true, nil)
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

func TestInteractiveShell_IO(t *testing.T) {
	t.Setenv("QSSH_KNOWN_HOSTS", filepath.Join(t.TempDir(), "known_hosts"))

	addr, _ := startTestSSHServer(t, func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		if c.User() == "testuser" && string(pass) == "testpass" {
			return nil, nil
		}
		return nil, fmt.Errorf("auth failed")
	})

	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{
		Name:     "test",
		Host:     host,
		User:     "testuser",
		Auth:     store.AuthPassword,
		Password: "testpass",
	}
	fmt.Sscanf(port, "%d", &p.Port)

	s, err := Dial(p, internal.NopProgress)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer s.Close()

	// Create pipes for interactive I/O (no real terminal needed)
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	defer stdinW.Close()
	defer stdoutR.Close()
	defer stderrR.Close()

	// Run InteractiveShell in background (it blocks on Wait)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.InteractiveShell(stdinR, stdoutW, stderrW, internal.NopProgress)
	}()

	// Wait for welcome message from the test SSH server
	buf := make([]byte, 256)
	_, err = stdoutR.Read(buf)
	if err != nil {
		t.Fatalf("read welcome: %v", err)
	}

	// Write a command and verify echo
	testCmd := "hello shell\n"
	stdinW.Write([]byte(testCmd))

	n, err := stdoutR.Read(buf)
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	output := string(buf[:n])
	if !strings.Contains(output, "hello shell") {
		t.Fatalf("expected echo %q in output, got %q", "hello shell", output)
	}

	// Close stdin to signal EOF, then wait for session to end
	stdinW.Close()
	if err := <-errCh; err != nil {
		t.Logf("Session ended: %v", err)
	}
}

func TestDial_PasswordAuth_Failure(t *testing.T) {
	t.Setenv("QSSH_KNOWN_HOSTS", filepath.Join(t.TempDir(), "known_hosts"))

	addr, _ := startTestSSHServer(t, func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		return nil, fmt.Errorf("auth failed")
	})

	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{
		Name: "test",
		Host: host,
		User: "testuser",
		Auth: store.AuthPassword,
		Password: "wrongpass",
	}
	fmt.Sscanf(port, "%d", &p.Port)

	_, err := Dial(p, nil)
	if err == nil {
		t.Fatal("expected error for bad password")
	}
	t.Logf("Got expected error: %v", err)
}

func TestDial_Timeout(t *testing.T) {
	p := store.Profile{
		Name: "test",
		Host: "203.0.113.1", // TEST-NET-3, unreachable
		Port: 22,
		User: "test",
		Auth: store.AuthPassword,
		Password: "test",
	}

	start := time.Now()
	_, err := Dial(p, nil)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
	t.Logf("Timeout error after %v: %v", duration, err)

	if duration > 15*time.Second {
		t.Fatalf("dial took too long (%v), timeout may not be working", duration)
	}
}

func TestAgentAuth_NoAgent(t *testing.T) {
	// Ensure no SSH agent socket is set
	t.Setenv("SSH_AUTH_SOCK", "")

	_, err := agentAuth()
	if err == nil {
		t.Fatal("expected error when SSH_AUTH_SOCK is not set")
	}
}

func TestExpandPath(t *testing.T) {
	if expandPath("~/.ssh/id_rsa") == "~/.ssh/id_rsa" {
		t.Fatal("expected ~ to be expanded")
	}
	if expandPath("/etc/hosts") != "/etc/hosts" {
		t.Fatal("expected absolute path to remain unchanged")
	}
	if expandPath("relative/path") != "relative/path" {
		t.Fatal("expected relative path to remain unchanged")
	}
}

func TestDial_PasswordAuth_Success(t *testing.T) {
	t.Setenv("QSSH_KNOWN_HOSTS", filepath.Join(t.TempDir(), "known_hosts"))

	addr, _ := startTestSSHServer(t, func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		if c.User() == "testuser" && string(pass) == "testpass" {
			return nil, nil
		}
		return nil, fmt.Errorf("auth failed")
	})

	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{
		Name:     "test",
		Host:     host,
		Port:     22,
		User:     "testuser",
		Auth:     store.AuthPassword,
		Password: "testpass",
	}
	fmt.Sscanf(port, "%d", &p.Port)

	progress := func(r internal.StepResult) {
		t.Logf("[progress] %s %s: %s", r.Status, r.ID, r.Message)
	}

	s, err := Dial(p, progress)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer s.Close()
}