package mount

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/pkg/sftp"
	"golang.org/x/net/webdav"

	"qssh/internal"
	"qssh/sshclient"
	"qssh/store"
)

// Mount connects to the profile, starts a WebDAV server, and mounts it
// via the system's WebDAV client. Blocks until interrupted.
func Mount(p store.Profile, mountPoint string) error {
	internal.RenderProfileHeader(p.Name, p.User, p.Host, p.Port)

	session, err := sshclient.Dial(p, internal.RenderProgress)
	if err != nil {
		return err
	}
	defer session.Close()

	// Create SFTP client from the existing SSH connection.
	internal.RenderProgress(internal.StepResult{
		ID: internal.StepAllocatePTY, Status: internal.StepRunning,
		Message: "Starting SFTP session",
	})
	sfClient, err := sftp.NewClient(session.Client())
	if err != nil {
		internal.RenderProgress(internal.StepResult{
			ID: internal.StepAllocatePTY, Status: internal.StepFailed,
			Message: fmt.Sprintf("SFTP failed: %v", err),
		})
		return fmt.Errorf("sftp client: %w", err)
	}
	defer sfClient.Close()
	internal.RenderProgress(internal.StepResult{
		ID: internal.StepAllocatePTY, Status: internal.StepDone,
		Message: "SFTP session established",
	})

	// Start WebDAV server on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("webdav listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	fs := &sftpFS{client: sfClient}
	handler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
	}

	httpServer := &http.Server{Handler: handler}
	go httpServer.Serve(listener)

	davURL := fmt.Sprintf("dav://127.0.0.1:%d", port)
	fmt.Fprintf(os.Stderr, "\n  WebDAV: %s\n", davURL)

	// Mount using platform-specific WebDAV client.
	mounted := false
	switch runtime.GOOS {
	case "linux":
		mounted = gvfsMount(davURL)
	case "darwin":
		mounted = macMount()
	case "windows":
		mounted = winMount(port, mountPoint)
	}

	if mounted {
		fmt.Fprintf(os.Stderr, "  Mounted. Press Ctrl+C to unmount.\n")
	} else {
		fmt.Fprintf(os.Stderr, "  Open in file manager: %s\n", davURL)
		if runtime.GOOS == "linux" {
			fmt.Fprintf(os.Stderr, "  Tip: install gvfs for auto-mount (sudo pacman -S gvfs)\n")
			fmt.Fprintf(os.Stderr, "  Or: gio mount %s\n", davURL)
		}
		fmt.Fprintf(os.Stderr, "  Press Ctrl+C to stop.\n")
	}

	// Wait for interrupt.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintf(os.Stderr, "\n  Cleaning up...\n")
	if mounted {
		gvfsUnmount(davURL)
	}
	httpServer.Shutdown(context.Background())
	return nil
}

// Unmount unmounts a WebDAV mount by URL or path.
func Unmount(target string) error {
	switch runtime.GOOS {
	case "linux":
		// Accept either dav:// URL or gvfs mount path.
		if strings.HasPrefix(target, "dav://") || strings.HasPrefix(target, "/") {
			return gvfsUnmount(target)
		}
		return gvfsUnmount("dav://" + target)
	case "darwin":
		return run("umount", target)
	case "windows":
		return run("net", "use", target, "/delete")
	default:
		return fmt.Errorf("unmount not supported on %s", runtime.GOOS)
	}
}

// --- Platform mount helpers ---

func gvfsMount(davURL string) bool {
	name := "gio"
	if _, err := exec.LookPath(name); err != nil {
		// fallback to deprecated gvfs-mount
		name = "gvfs-mount"
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
		return exec.Command(name, davURL).Start() == nil
	}
	// gio mount dav://127.0.0.1:PORT
	// mounts to /run/user/$UID/gvfs/...
	cmd := exec.Command(name, "mount", davURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  gio mount failed: %s\n", strings.TrimSpace(string(out)))
		return false
	}
	return true
}

func gvfsUnmount(davURL string) error {
	if _, err := exec.LookPath("gio"); err == nil {
		return run("gio", "mount", "-u", davURL)
	}
	return run("gvfs-mount", "-u", davURL)
}

func macMount() bool {
	// macOS Finder supports WebDAV via http:// in "Connect to Server".
	// `open http://...` opens the browser, which is wrong.
	// Just print instructions for now.
	return false
}

func winMount(port int, drive string) bool {
	if drive == "" {
		drive = "Z:"
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	return run("net", "use", drive, url) == nil
}

// --- Utilities ---

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