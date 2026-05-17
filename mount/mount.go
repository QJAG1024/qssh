package mount

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/net/webdav"

	"qssh/internal"
	"qssh/sshclient"
	"qssh/store"
)

// --- State file ---

type mountEntry struct {
	Port    int    `json:"port"`
	PID     int    `json:"pid"`
	DavURL  string `json:"dav_url"`
	Status  string `json:"status"` // "starting", "ready", "failed"
	Message string `json:"message,omitempty"`
}

func statePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "qssh", "mounts.json")
}

var stateMu sync.Mutex

func loadState() map[string]mountEntry {
	stateMu.Lock()
	defer stateMu.Unlock()
	data, err := os.ReadFile(statePath())
	if err != nil {
		return map[string]mountEntry{}
	}
	var m map[string]mountEntry
	json.Unmarshal(data, &m)
	if m == nil {
		m = map[string]mountEntry{}
	}
	return m
}

func saveState(m map[string]mountEntry) {
	stateMu.Lock()
	defer stateMu.Unlock()
	data, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(statePath(), data, 0600)
}

// --- Mount (launcher, foreground) ---

// Mount forks a daemon to serve WebDAV and exits immediately.
func Mount(name, mountPoint string) error {
	state := loadState()
	if _, exists := state[name]; exists {
		return fmt.Errorf("profile %q is already mounted", name)
	}

	// Pick a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("pick port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Write initial state before forking.
	davURL := fmt.Sprintf("dav://127.0.0.1:%d", port)
	state[name] = mountEntry{
		Port:    port,
		DavURL:  davURL,
		Status:  "starting",
		Message: "Starting...",
	}
	saveState(state)

	// Fork daemon — re-exec self with hidden flag.
	cmd := exec.Command(os.Args[0], "--mount-daemon", name, "--port", fmt.Sprintf("%d", port))
	cmd.SysProcAttr = daemonSysProcAttr()
	cmd.Stderr = nil // detach stderr
	cmd.Stdout = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		// Clean up state on fork failure.
		delete(state, name)
		saveState(state)
		return fmt.Errorf("fork daemon: %w", err)
	}

	// Wait for daemon to reach "ready".
	lastMsg := ""
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		st := loadState()
		entry, ok := st[name]
		if !ok {
			break // cleaned up — daemon likely failed
		}
		// Print new progress messages from daemon.
		if entry.Message != "" && entry.Message != lastMsg {
			fmt.Fprintln(os.Stderr, "  → "+entry.Message)
			lastMsg = entry.Message
		}
		switch entry.Status {
		case "ready":
			display := davURL
			if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
				display = fmt.Sprintf("http://127.0.0.1:%d", entry.Port)
			}
			fmt.Printf("Mounted: %s\n", display)
			return nil
		case "failed":
			return fmt.Errorf("daemon failed")
		}
	}

	// Timeout — clean up orphaned daemon and its state.
	st := loadState()
	if entry, ok := st[name]; ok {
		if proc, err := os.FindProcess(entry.PID); err == nil {
			proc.Kill()
		}
		delete(st, name)
		saveState(st)
	}
	return fmt.Errorf("daemon did not become ready in time")
}

// --- MountDaemon (background worker) ---

// MountDaemon is the hidden entry point for the mount worker process.
// It is called via --mount-daemon after forking.
func MountDaemon(profileName, portStr string) {
	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		os.Exit(1)
	}

	setProgress(profileName, "Opening store...")
	openStore, err := openStoreFn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mount-daemon] open store: %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}

	p, exists := openStore.Get(profileName)
	if !exists {
		fmt.Fprintf(os.Stderr, "[mount-daemon] profile %q not found\n", profileName)
		setFailed(profileName)
		os.Exit(1)
	}

	// Dial SSH.
	setProgress(profileName, "Connecting SSH...")
	session, err := sshclient.Dial(p, internal.NopProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mount-daemon] SSH dial: %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}
	defer session.Close()

	// SFTP client.
	setProgress(profileName, "Starting SFTP...")
	sfClient, err := sftp.NewClient(session.Client())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mount-daemon] SFTP: %v\n", err)
		setFailed(profileName)
		os.Exit(1)
	}
	defer sfClient.Close()

	// Start WebDAV server.
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		setFailed(profileName)
		os.Exit(1)
	}

	fs := &sftpFS{client: sfClient}
	handler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}
	httpServer := &http.Server{Handler: handler}
	go httpServer.Serve(listener)

	// Platform-specific mount.
	// Linux (GVFS) uses dav:// scheme; macOS/Windows use http://.
	davURL := fmt.Sprintf("dav://127.0.0.1:%d", port)
	httpURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if runtime.GOOS == "linux" { setProgress(profileName, "Mounting via gio...") }
	mounted := false
	switch runtime.GOOS {
	case "linux":
		mounted = linuxMount(davURL)
	case "darwin":
		mounted = macMount(httpURL)
	case "windows":
		mounted = winMount(port, "")
	}

	// Mark ready.
	markReady(profileName, port, davURL, os.Getpid())

	// Handle SIGTERM for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		if mounted {
			cleanupMount(profileName, davURL)
		}
		httpServer.Shutdown(context.Background())
		os.Exit(0)
	}()

	// Block forever (HTTP server runs in goroutine).
	select {}
}

// --- Unmount ---

// Unmount stops a mount by profile name.
func Unmount(name string) error {
	state := loadState()
	entry, exists := state[name]
	if !exists {
		return fmt.Errorf("profile %q is not mounted", name)
	}

	// Unmount first, then kill.
	cleanupMount(name, entry.DavURL)

	// Kill daemon.
	proc, err := os.FindProcess(entry.PID)
	if err == nil {
		proc.Signal(syscall.SIGTERM)
		// Give it a moment, then force kill.
		time.Sleep(200 * time.Millisecond)
		proc.Kill()
	}

	delete(state, name)
	saveState(state)
	return nil
}

// --- Internal helpers ---

func cleanupMount(name, davURL string) {
	switch runtime.GOOS {
	case "linux":
		linuxUnmount(davURL)
	case "darwin":
		// Mac user unmounts via Finder.
	case "windows":
		// net use /delete handled by daemon's cleanup.
	}
}

func markReady(name string, port int, davURL string, pid int) {
	state := loadState()
	state[name] = mountEntry{
		Port:    port,
		PID:     pid,
		DavURL:  davURL,
		Status:  "ready",
		Message: "",
	}
	saveState(state)
}

func setProgress(name, msg string) {
	state := loadState()
	if entry, ok := state[name]; ok {
		entry.Message = msg
		state[name] = entry
		saveState(state)
	}
}

func setFailed(name string) {
	state := loadState()
	delete(state, name)
	saveState(state)
}

// openStoreFn is a package-level hook so the daemon can open the store
// without importing the cmd package (would create a cycle).
// Set once at startup.
var openStoreFn func() (*store.Store, error) = nil

// SetOpenStore provides the store-opener function from cmd package.
func SetOpenStore(fn func() (*store.Store, error)) {
	openStoreFn = fn
}

// --- Platform mount helpers ---

// backendCmd describes a WebDAV mount/unmount tool.
type backendCmd struct {
	Name       string
	MountArg   string // arg after "mount" subcommand, or "" if no subcommand
	UnmountArg string
}

// linuxBackends returns the ordered list of backends to try, based on
// XDG_CURRENT_DESKTOP and the mount.backend config override.
func linuxBackends() []backendCmd {
	// Config override.
	cfg := internal.OpenConfig(internal.DefaultConfigPath())
	switch cfg.Get("mount.backend") {
	case "gio":
		return []backendCmd{
			{Name: "gio", MountArg: "mount", UnmountArg: "-u"},
		}
	case "kio":
		return []backendCmd{
			{Name: "kioclient6", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient5", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient", MountArg: "mount", UnmountArg: "unmount"},
		}
	}

	// DE detection.
	de := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	switch {
	case strings.Contains(de, "kde"):
		return []backendCmd{
			{Name: "kioclient6", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient5", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "gio", MountArg: "mount", UnmountArg: "-u"},
		}
	case de == "gnome" || de == "cinnamon" || strings.Contains(de, "xfce") ||
		de == "budgie" || de == "pantheon" || de == "mate" || de == "lxde":
		return []backendCmd{
			{Name: "gio", MountArg: "mount", UnmountArg: "-u"},
			{Name: "kioclient6", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient5", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient", MountArg: "mount", UnmountArg: "unmount"},
		}
	default:
		// Unknown / tiling WM / none — try everything.
		return []backendCmd{
			{Name: "gio", MountArg: "mount", UnmountArg: "-u"},
			{Name: "kioclient6", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient5", MountArg: "mount", UnmountArg: "unmount"},
			{Name: "kioclient", MountArg: "mount", UnmountArg: "unmount"},
		}
	}
}

func linuxMount(davURL string) bool {
	for _, b := range linuxBackends() {
		if _, err := exec.LookPath(b.Name); err != nil {
			continue
		}
		args := []string{}
		if b.MountArg != "" {
			args = append(args, b.MountArg)
		}
		args = append(args, davURL)
		cmd := exec.Command(b.Name, args...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return true
		}
		fmt.Fprintf(os.Stderr, "  %s mount failed: %s\n", b.Name, strings.TrimSpace(string(out)))
	}
	return false
}

func linuxUnmount(davURL string) error {
	for _, b := range linuxBackends() {
		if _, err := exec.LookPath(b.Name); err != nil {
			continue
		}
		args := []string{}
		if b.UnmountArg != "" {
			args = append(args, b.UnmountArg)
		}
		args = append(args, davURL)
		out, err := exec.Command(b.Name, args...).CombinedOutput()
		if err == nil {
			return nil
		}
		return fmt.Errorf("%s unmount failed: %w\n%s", b.Name, err, out)
	}
	return fmt.Errorf("no mount backend found")
}

func macMount(httpURL string) bool {
	// macOS Finder mounts WebDAV via http://.
	script := fmt.Sprintf(
		`tell application "Finder" to mount volume "%s"`, httpURL)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Start() == nil
}

func winMount(port int, drive string) bool {
	if drive == "" {
		drive = "Z:"
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	return run("net", "use", drive, url) == nil
}

func run(name string, arg ...string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found", name)
	}
	out, err := exec.Command(name, arg...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w\n%s", name, err, out)
	}
	return nil
}