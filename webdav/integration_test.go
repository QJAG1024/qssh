//go:build !windows

package webdav

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
)

// startInMemorySFTP spins up an in-process SFTP server over io.Pipe
// (pkg/sftp's own test pattern — no SSH, no network, CI-safe). The server
// serves the real local filesystem; tests use absolute tempdir paths.
func startInMemorySFTP(t *testing.T) *sftp.Client {
	t.Helper()
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	srv, err := sftp.NewServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	client, err := sftp.NewClientPipe(cr, cw)
	if err != nil {
		t.Fatalf("sftp client: %v", err)
	}
	// io.Pipe sftp Close blocks (no proper EOF semantics), so close in a
	// goroutine and don't wait.
	t.Cleanup(func() { go client.Close() })
	return client
}

func newTestServer(t *testing.T) (*Server, string) {
	root := t.TempDir()
	// Create a known file and subdir.
	os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello world"), 0644)
	os.MkdirAll(filepath.Join(root, "subdir"), 0755)
	os.WriteFile(filepath.Join(root, "subdir", "inner.txt"), []byte("inner"), 0644)

	client := startInMemorySFTP(t)
	srv := New(client)
	return srv, root
}

func TestWebDAV_PropfindLists(t *testing.T) {
	srv, root := newTestServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(urlFor(ts, root)) // GET dir shows listing
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET root = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello.txt") {
		t.Errorf("dir listing missing hello.txt: %s", body)
	}
}

func TestWebDAV_GetFile(t *testing.T) {
	srv, root := newTestServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(urlFor(ts, filepath.Join(root, "hello.txt")))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello world" {
		t.Errorf("GET hello.txt = %q, want 'hello world'", body)
	}
	if resp.Header.Get("Content-Length") != "11" {
		t.Errorf("Content-Length = %q, want 11", resp.Header.Get("Content-Length"))
	}
}

func TestWebDAV_Range(t *testing.T) {
	srv, root := newTestServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, _ := http.NewRequest("GET", urlFor(ts, filepath.Join(root, "hello.txt")), nil)
	req.Header.Set("Range", "bytes=6-10")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 206 {
		t.Fatalf("Range = %d, want 206", resp.StatusCode)
	}
	if string(body) != "world" {
		t.Errorf("Range body = %q, want 'world'", body)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "bytes 6-10/11" {
		t.Errorf("Content-Range = %q, want 'bytes 6-10/11'", cr)
	}
}

func TestWebDAV_PutAndGet(t *testing.T) {
	srv, root := newTestServer(t)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// PUT a new file.
	req, _ := http.NewRequest("PUT", urlFor(ts, filepath.Join(root, "uploaded.txt")), strings.NewReader("uploaded data"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("PUT = %d, want 201", resp.StatusCode)
	}
	// Verify on disk.
	data, err := os.ReadFile(filepath.Join(root, "uploaded.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "uploaded data" {
		t.Errorf("uploaded = %q", data)
	}
	// GET it back.
	resp2, err := http.Get(urlFor(ts, filepath.Join(root, "uploaded.txt")))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	if string(body) != "uploaded data" {
		t.Errorf("GET after PUT = %q", body)
	}
}

func TestWebDAV_TokenAuth(t *testing.T) {
	srv, root := newTestServer(t)
	srv.SetToken("secret-token")
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// No token -> 401.
	resp, _ := http.Get(urlFor(ts, filepath.Join(root, "hello.txt")))
	if resp.StatusCode != 401 {
		t.Errorf("no token = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
	// Header token -> 200.
	req, _ := http.NewRequest("GET", urlFor(ts, filepath.Join(root, "hello.txt")), nil)
	req.Header.Set("X-QSSH-Token", "secret-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		t.Errorf("header token = %d, want 200", resp2.StatusCode)
	}
	resp2.Body.Close()
	// Query token -> 200.
	resp3, _ := http.Get(urlFor(ts, filepath.Join(root, "hello.txt")) + "?token=secret-token")
	if resp3.StatusCode != 200 {
		t.Errorf("query token = %d, want 200", resp3.StatusCode)
	}
	resp3.Body.Close()
}

func TestWebDAV_StateFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QSSH_WEBDAV_STATE", filepath.Join(dir, "webdav.json"))
	// saveState + loadState roundtrip.
	m := map[string]entry{"p": {Port: 1234, PID: 42, URL: "http://127.0.0.1:1234/", Status: "ready"}}
	saveState(m)
	if _, err := os.Stat(statePath()); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
	got := loadState()
	if got["p"].Port != 1234 || got["p"].Status != "ready" {
		t.Errorf("state roundtrip = %+v", got["p"])
	}
	_ = json.Valid
}

// urlFor returns the server URL for an absolute tempdir path.
func urlFor(ts *httptest.Server, abs string) string {
	return ts.URL + abs
}

func TestWebDAV_Readonly(t *testing.T) {
	srv, root := newTestServer(t)
	srv.SetReadonly(true)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// GET still works.
	resp, _ := http.Get(urlFor(ts, filepath.Join(root, "hello.txt")))
	if resp.StatusCode != 200 {
		t.Errorf("readonly GET = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// PUT rejected.
	req, _ := http.NewRequest("PUT", urlFor(ts, filepath.Join(root, "nope.txt")), strings.NewReader("x"))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Errorf("readonly PUT = %d, want 403", resp2.StatusCode)
	}
	// File not created.
	if _, err := os.Stat(filepath.Join(root, "nope.txt")); err == nil {
		t.Error("readonly PUT should not create file")
	}
}
