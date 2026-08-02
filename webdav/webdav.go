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
	DisplayName     string       `xml:"D:displayname,omitempty"`
	GetContentLength *int64      `xml:"D:getcontentlength,omitempty"`
	GetLastModified string       `xml:"D:getlastmodified,omitempty"`
	GetContentType  string       `xml:"D:getcontenttype,omitempty"`
	ResourceType    resourceType `xml:"D:resourcetype"`
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
	case "OPTIONS":
		w.Header().Set("Allow", "PROPFIND, GET, HEAD, PUT, MKCOL, DELETE, OPTIONS")
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

	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	if ct := mime.TypeByExtension(filepath.Ext(fi.Name())); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Last-Modified", fi.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	if r.Method == "HEAD" {
		w.WriteHeader(http.StatusOK)
		return
	}
	io.Copy(w, f)
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
	if _, err := io.Copy(f, r.Body); err != nil {
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

