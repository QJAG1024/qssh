// Package webdav serves an SFTP-backed WebDAV endpoint. PROPFIND uses a single
// ReadDir to build the response (no per-entry round-trips — see
// MOUNT_EXPERIMENTS_REPORT.md for why this matters on high-latency links).
package webdav

import (
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/sftp"
)

// Server serves an SFTP-backed WebDAV endpoint over HTTP.
type Server struct {
	client *sftp.Client
	mu     sync.Mutex
}

// New creates a WebDAV server wrapping the SFTP client.
func New(client *sftp.Client) *Server {
	return &Server{client: client}
}

// --- WebDAV XML types (minimal RFC 4918) ---

type multistatus struct {
	XMLName   xml.Name   `xml:"D:multistatus"`
	XmlnsD    string     `xml:"xmlns:D,attr"`
	Responses []response `xml:"D:response"`
}

type response struct {
	Href     string     `xml:"D:href"`
	Propstat []propstat `xml:"D:propstat"`
}

type propstat struct {
	Prop   prop   `xml:"D:prop"`
	Status string `xml:"D:status"`
}

type prop struct {
	DisplayName      string       `xml:"D:displayname,omitempty"`
	GetContentLength *int64       `xml:"D:getcontentlength,omitempty"`
	GetLastModified  string       `xml:"D:getlastmodified,omitempty"`
	GetContentType   string       `xml:"D:getcontenttype,omitempty"`
	ResourceType     resourceType `xml:"D:resourcetype"`
}

type resourceType struct {
	Collection *struct{} `xml:"D:collection,omitempty"`
}

const (
	xmlnsDAV   = "DAV:"
	httpStatus = "HTTP/1.1 200 OK"
	http404    = "HTTP/1.1 404 Not Found"
)

// ServeHTTP routes WebDAV methods.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Normalize path: WebDAV URLs are rooted at /, mapping to SFTP paths.
	p := r.URL.Path
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	switch r.Method {
	case "PROPFIND":
		s.handlePropfind(w, r, p)
	case "GET", "HEAD":
		s.handleGet(w, r, p)
	case "PUT":
		s.handlePut(w, r, p)
	case "MKCOL":
		s.handleMkcol(w, r, p)
	case "DELETE":
		s.handleDelete(w, r, p)
	case "MOVE":
		s.handleMove(w, r, p)
	case "COPY":
		s.handleCopy(w, r, p)
	case "LOCK":
		s.handleLock(w, r, p)
	case "UNLOCK":
		s.handleUnlock(w, r, p)
	case "OPTIONS":
		w.Header().Set("Allow", "PROPFIND, GET, HEAD, PUT, MKCOL, DELETE, MOVE, COPY, LOCK, UNLOCK, OPTIONS")
		w.Header().Set("DAV", "1")
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- PROPFIND: single ReadDir, no per-entry round-trips ---

func (s *Server) handlePropfind(w http.ResponseWriter, r *http.Request, p string) {
	// Depth header: "0" (self only) or "1" (children). We support both;
	// deeper is ignored (rarely used by clients).
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}

	entries, err := s.client.ReadDir(p)
	isRoot := p == "/" || p == ""
	if err != nil && !isRoot {
		// Maybe it's a file, not a dir.
		if fi, serr := s.client.Stat(p); serr == nil {
			ms := multistatus{XmlnsD: xmlnsDAV}
			ms.Responses = append(ms.Responses, makeResponse(p, fi))
			writeMultistatus(w, ms)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	ms := multistatus{XmlnsD: xmlnsDAV}
	// Self entry: stat the dir.
	if fi, serr := s.client.Stat(p); serr == nil {
		ms.Responses = append(ms.Responses, makeResponse(withSlash(p), fi))
	}
	if depth == "1" && err == nil {
		for _, e := range entries {
			child := path.Join(p, e.Name())
			ms.Responses = append(ms.Responses, makeResponse(child, e))
		}
	}
	writeMultistatus(w, ms)
}

func makeResponse(href string, fi os.FileInfo) response {
	rs := response{Href: href}
	ps := propstat{Status: httpStatus}
	ps.Prop.DisplayName = path.Base(href)
	if fi.IsDir() {
		ps.Prop.ResourceType.Collection = &struct{}{}
	} else {
		sz := fi.Size()
		ps.Prop.GetContentLength = &sz
		ct := mime.TypeByExtension(filepath.Ext(fi.Name()))
		if ct != "" {
			ps.Prop.GetContentType = ct
		}
	}
	ps.Prop.GetLastModified = fi.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	rs.Propstat = []propstat{ps}
	return rs
}

func withSlash(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	return strings.TrimSuffix(p, "/") + "/"
}

func writeMultistatus(w http.ResponseWriter, ms multistatus) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(ms)
}

// --- GET/HEAD: stream file ---

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, p string) {
	fi, err := s.client.Stat(p)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if fi.IsDir() {
		// gvfs/Finder may GET a dir; return a simple listing.
		entries, err := s.client.ReadDir(p)
		if err == nil {
			fmt.Fprintf(w, "<html><body><h1>%s</h1><ul>", p)
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				fmt.Fprintf(w, "<li><a href=\"%s\">%s</a></li>", path.Join(p, e.Name()), name)
			}
			fmt.Fprintf(w, "</ul></body></html>")
			return
		}
		http.Error(w, "readdir error", http.StatusInternalServerError)
		return
	}

	f, err := s.client.Open(p)
	if err != nil {
		http.Error(w, "open error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	size := fi.Size()
	if ct := mime.TypeByExtension(filepath.Ext(fi.Name())); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Last-Modified", fi.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	// Range support (bytes=start-end) for streaming/large files.
	if rng := r.Header.Get("Range"); rng != "" && strings.HasPrefix(rng, "bytes=") {
		if start, end, ok := parseRange(rng, size); ok {
			if _, err := f.Seek(start, io.SeekStart); err != nil {
				http.Error(w, "seek error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
			w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
			w.WriteHeader(http.StatusPartialContent)
			if r.Method == "HEAD" {
				return
			}
			io.CopyN(w, f, end-start+1)
			return
		}
		// Malformed range: 416 with Accept-Ranges.
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	if r.Method == "HEAD" {
		w.WriteHeader(http.StatusOK)
		return
	}
	io.Copy(w, f)
}

// parseRange parses "bytes=start-end" against size. start inclusive, end
// inclusive (defaults: start=0, end=size-1; suffix form bytes=-N supported).
func parseRange(rng string, size int64) (start, end int64, ok bool) {
	spec := strings.TrimPrefix(rng, "bytes=")
	if i := strings.Index(spec, ","); i >= 0 {
		spec = spec[:i] // single range only
	}
	spec = strings.TrimSpace(spec)
	if spec == "" || size <= 0 {
		return 0, 0, false
	}
	if strings.HasPrefix(spec, "-") {
		// suffix: bytes=-N means last N bytes.
		n, err := strconv.ParseInt(strings.TrimPrefix(spec, "-"), 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	}
	dash := strings.Index(spec, "-")
	if dash < 0 {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(spec[:dash]), 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	endStr := strings.TrimSpace(spec[dash+1:])
	if endStr == "" {
		end = size - 1
	} else {
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true
}

// --- PUT: write file ---

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request, p string) {
	// Ensure parent dir exists.
	parent := path.Dir(p)
	if _, err := s.client.Stat(parent); err != nil {
		http.Error(w, "parent not found", http.StatusConflict)
		return
	}
	f, err := s.client.Create(p)
	if err != nil {
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	// ReadFromWithConcurrency is essential on high-latency links: plain
	// io.Copy falls back to sequential Write (one ACK round-trip per packet),
	// making uploads ~30x slower than downloads. Force concurrent writes.
	if _, err := f.ReadFromWithConcurrency(r.Body, 16); err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// --- MKCOL: make directory ---

func (s *Server) handleMkcol(w http.ResponseWriter, r *http.Request, p string) {
	if err := s.client.Mkdir(p); err != nil {
		http.Error(w, "mkdir error", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// --- DELETE ---

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, p string) {
	if err := s.client.RemoveAll(p); err != nil {
		http.Error(w, "delete error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- MOVE / COPY ---

// destPath extracts the Destination header, URL-decoded, rooted at /.
func destPath(r *http.Request) (string, bool) {
	dest := r.Header.Get("Destination")
	if dest == "" {
		return "", false
	}
	// Strip scheme://host:port prefix if present.
	if i := strings.Index(dest, "://"); i >= 0 {
		rest := dest[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			dest = rest[j:]
		} else {
			dest = "/"
		}
	}
	if !strings.HasPrefix(dest, "/") {
		dest = "/" + dest
	}
	return dest, true
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request, src string) {
	dest, ok := destPath(r)
	if !ok {
		http.Error(w, "missing Destination", http.StatusBadRequest)
		return
	}
	if err := s.client.Rename(src, dest); err != nil {
		http.Error(w, "rename failed", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleCopy copies src to dest recursively via SFTP (no server-side COPY in
// pkg/sftp, so walk + stream).
func (s *Server) handleCopy(w http.ResponseWriter, r *http.Request, src string) {
	dest, ok := destPath(r)
	if !ok {
		http.Error(w, "missing Destination", http.StatusBadRequest)
		return
	}
	if err := s.copyRecursive(src, dest); err != nil {
		http.Error(w, "copy failed", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) copyRecursive(src, dest string) error {
	fi, err := s.client.Stat(src)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		in, err := s.client.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := s.client.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := out.ReadFromWithConcurrency(in, 16); err != nil {
			return err
		}
		return nil
	}
	// Directory: mkdir dest, recurse children.
	if err := s.client.Mkdir(dest); err != nil && !isExistsErr(err) {
		return err
	}
	entries, err := s.client.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := s.copyRecursive(
			path.Join(src, e.Name()),
			path.Join(dest, e.Name()),
		); err != nil {
			return err
		}
	}
	return nil
}

func isExistsErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "File exists")
}

// --- LOCK / UNLOCK (no-op locks: always succeed, nothing held) ---

type lockDiscovery struct {
	XMLName       xml.Name `xml:"D:prop"`
	LockDiscovery *string  `xml:"D:lockdiscovery,omitempty"`
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request, p string) {
	// We don't hold locks, but reply with an empty lockdiscovery so clients
	// that require LOCK (e.g. LibreOffice/Word) proceed without error.
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Lock-Token", "<urn:qssh:lock:0>")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<D:prop xmlns:D="DAV:"><D:lockdiscovery/></D:prop>`)
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request, p string) {
	w.WriteHeader(http.StatusNoContent)
}

// --- Range support in GET ---
