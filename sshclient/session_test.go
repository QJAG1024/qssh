package sshclient

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"qssh/internal"
	"qssh/store"
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
// testConfigDir isolates the test from the user's real ~/.config.
func testConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

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
	testConfigDir(t)
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
	testConfigDir(t)
	t.Setenv("QSSH_KNOWN_HOSTS", filepath.Join(t.TempDir(), "known_hosts"))

	addr, _ := startTestSSHServer(t, func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		return nil, fmt.Errorf("auth failed")
	})

	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{
		Name:     "test",
		Host:     host,
		User:     "testuser",
		Auth:     store.AuthPassword,
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
	testConfigDir(t)
	p := store.Profile{
		Name:     "test",
		Host:     "203.0.113.1", // TEST-NET-3, unreachable
		Port:     22,
		User:     "test",
		Auth:     store.AuthPassword,
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
	testConfigDir(t)
	// Ensure no SSH agent socket is set
	t.Setenv("SSH_AUTH_SOCK", "")

	_, err := agentAuth()
	if err == nil {
		t.Fatal("expected error when SSH_AUTH_SOCK is not set")
	}
}

func TestExpandPath(t *testing.T) {
	testConfigDir(t)
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
	testConfigDir(t)
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

// generateHostKey returns a fresh SSH host signer for TOFU tests.
func generateHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	return signer
}

// startTestSSHServerWithKey starts a server with a specific host key.
func startTestSSHServerWithKey(t *testing.T, hostKey ssh.Signer, passwordCallback func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error)) string {
	t.Helper()

	config := &ssh.ServerConfig{
		PasswordCallback: passwordCallback,
	}
	config.AddHostKey(hostKey)

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
					ch.Write([]byte("Welcome to test SSH server\r\n"))
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

	return listener.Addr().String()
}

func authOK(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	if c.User() == "testuser" && string(pass) == "testpass" {
		return nil, nil
	}
	return nil, fmt.Errorf("auth failed")
}

func TestHostKey_TOFU_AcceptsAndPersists(t *testing.T) {
	testConfigDir(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	t.Setenv("QSSH_KNOWN_HOSTS", kh)

	addr := startTestSSHServerWithKey(t, generateHostKey(t), authOK)
	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{Name: "tofu", Host: host, Port: 22, User: "testuser", Auth: store.AuthPassword, Password: "testpass"}
	fmt.Sscanf(port, "%d", &p.Port)

	if _, err := Dial(p, nil); err != nil {
		t.Fatalf("first dial failed: %v", err)
	}

	data, err := os.ReadFile(kh)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("known_hosts is empty after TOFU accept")
	}

	// Second dial to the same host:port/key should succeed.
	_, err = Dial(p, nil)
	if err != nil {
		t.Fatalf("second dial failed: %v", err)
	}
}

func TestHostKey_MismatchRejected(t *testing.T) {
	testConfigDir(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	t.Setenv("QSSH_KNOWN_HOSTS", kh)

	// Start a server on a fixed port, connect to persist key, then close it.
	config1 := &ssh.ServerConfig{PasswordCallback: authOK}
	config1.AddHostKey(generateHostKey(t))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	stop := make(chan struct{})
	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				select {
				case <-stop:
					return
				default:
					return
				}
			}
			go func(c net.Conn) {
				_, chans, reqs, err := ssh.NewServerConn(c, config1)
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
				}
			}(nConn)
		}
	}()

	addr := listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{Name: "tofu", Host: host, Port: 22, User: "testuser", Auth: store.AuthPassword, Password: "testpass"}
	fmt.Sscanf(port, "%d", &p.Port)

	if _, err := Dial(p, nil); err != nil {
		t.Fatalf("first dial failed: %v", err)
	}

	// Close the first server and start a new one with a different key on the same port.
	close(stop)
	listener.Close()
	// Give the OS a moment to release the port.
	time.Sleep(50 * time.Millisecond)

	config2 := &ssh.ServerConfig{PasswordCallback: authOK}
	config2.AddHostKey(generateHostKey(t))
	listener2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listen second server: %v", err)
	}
	defer listener2.Close()
	go func() {
		for {
			nConn, err := listener2.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_, chans, reqs, err := ssh.NewServerConn(c, config2)
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
				}
			}(nConn)
		}
	}()

	_, err = Dial(p, nil)
	if err == nil {
		t.Fatal("expected failure when host key changed")
	}
	if !strings.Contains(err.Error(), "mismatch") && !strings.Contains(err.Error(), "key") {
		t.Fatalf("expected key mismatch error, got: %v", err)
	}
}

func TestHostKey_KnownHostsNotWritable(t *testing.T) {
	testConfigDir(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	// Make known_hosts a directory so the file write fails.
	if err := os.Mkdir(kh, 0500); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QSSH_KNOWN_HOSTS", kh)

	addr := startTestSSHServerWithKey(t, generateHostKey(t), authOK)
	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{Name: "tofu", Host: host, Port: 22, User: "testuser", Auth: store.AuthPassword, Password: "testpass"}
	fmt.Sscanf(port, "%d", &p.Port)

	_, err := Dial(p, nil)
	if err == nil {
		t.Fatal("expected failure when known_hosts not writable")
	}
}

func TestHostKey_PortIsolation(t *testing.T) {
	testConfigDir(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	t.Setenv("QSSH_KNOWN_HOSTS", kh)

	// Two servers on different ports with different host keys.
	addr1 := startTestSSHServerWithKey(t, generateHostKey(t), authOK)
	addr2 := startTestSSHServerWithKey(t, generateHostKey(t), authOK)

	parse := func(a string) store.Profile {
		host, port, _ := net.SplitHostPort(a)
		p := store.Profile{Name: "tofu", Host: host, User: "testuser", Auth: store.AuthPassword, Password: "testpass"}
		fmt.Sscanf(port, "%d", &p.Port)
		return p
	}

	p1 := parse(addr1)
	p2 := parse(addr2)

	if _, err := Dial(p1, nil); err != nil {
		t.Fatalf("dial port1 failed: %v", err)
	}
	if _, err := Dial(p2, nil); err != nil {
		t.Fatalf("dial port2 failed: %v", err)
	}

	// Both keys should be present independently.
	lines, err := os.ReadFile(kh)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if strings.Count(string(lines), "[") < 2 {
		t.Fatalf("expected two independent host entries, got:\n%s", string(lines))
	}
}

func TestHostKey_ConcurrentFirstUseOnlyAcceptsOne(t *testing.T) {
	testConfigDir(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	t.Setenv("QSSH_KNOWN_HOSTS", kh)

	key1 := generateHostKey(t)
	key2 := generateHostKey(t)

	addr1 := startTestSSHServerWithKey(t, key1, authOK)
	addr2 := startTestSSHServerWithKey(t, key2, authOK)

	parse := func(a string) store.Profile {
		host, port, _ := net.SplitHostPort(a)
		p := store.Profile{Name: "tofu", Host: host, User: "testuser", Auth: store.AuthPassword, Password: "testpass"}
		fmt.Sscanf(port, "%d", &p.Port)
		return p
	}

	p1 := parse(addr1)
	p2 := parse(addr2)

	// Race two writers writing to the same known_hosts file for different
	// host:port entries. The lock should serialize them and both succeed.
	var wg sync.WaitGroup
	wg.Add(2)
	var errs [2]error
	go func() {
		defer wg.Done()
		_, errs[0] = Dial(p1, nil)
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = Dial(p2, nil)
	}()
	wg.Wait()

	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent first use failed: %v, %v", errs[0], errs[1])
	}
}

func TestHostKey_StrictRejectsUnknown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	os.Unsetenv("XDG_CONFIG_HOME")
	t.Setenv("QSSH_KNOWN_HOSTS", filepath.Join(dir, "known_hosts"))

	cfgDir, _ := os.UserConfigDir()
	cfgDir = filepath.Join(cfgDir, "qssh")
	os.MkdirAll(cfgDir, 0700)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"hostkey.mode":"strict"}`), 0600); err != nil {
		t.Fatal(err)
	}

	addr := startTestSSHServerWithKey(t, generateHostKey(t), authOK)
	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{Name: "strict", Host: host, Port: 22, User: "testuser", Auth: store.AuthPassword, Password: "testpass"}
	fmt.Sscanf(port, "%d", &p.Port)

	_, err := Dial(p, nil)
	if err == nil {
		t.Fatal("expected strict mode to reject unknown host")
	}
}

func TestHostKey_CorruptConfigFailsClosed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	os.Unsetenv("XDG_CONFIG_HOME")
	t.Setenv("QSSH_KNOWN_HOSTS", filepath.Join(dir, "known_hosts"))

	cfgDir, _ := os.UserConfigDir()
	cfgDir = filepath.Join(cfgDir, "qssh")
	os.MkdirAll(cfgDir, 0700)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	addr := startTestSSHServerWithKey(t, generateHostKey(t), authOK)
	host, port, _ := net.SplitHostPort(addr)
	p := store.Profile{Name: "tofu", Host: host, Port: 22, User: "testuser", Auth: store.AuthPassword, Password: "testpass"}
	fmt.Sscanf(port, "%d", &p.Port)

	_, err := Dial(p, nil)
	if err == nil {
		t.Fatal("expected failure with corrupt hostkey config")
	}
}
