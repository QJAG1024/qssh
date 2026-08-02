package webdav

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMakeResponseFile(t *testing.T) {
	fi := fakeInfo{name: "test.txt", size: 42, dir: false, mod: time.Now()}
	rs := makeResponse("/etc/test.txt", fi)
	if len(rs.Propstat) != 1 {
		t.Fatal("want 1 propstat")
	}
	if rs.Propstat[0].Status != "HTTP/1.1 200 OK" {
		t.Errorf("status = %q", rs.Propstat[0].Status)
	}
	if rs.Propstat[0].Prop.GetContentLength == nil || *rs.Propstat[0].Prop.GetContentLength != 42 {
		t.Error("getcontentlength should be 42")
	}
	if rs.Propstat[0].Prop.ResourceType.Collection != nil {
		t.Error("file must not have collection resourcetype")
	}
}

func TestMakeResponseDir(t *testing.T) {
	fi := fakeInfo{name: "dir", size: 0, dir: true, mod: time.Now()}
	rs := makeResponse("/etc/dir/", fi)
	if rs.Propstat[0].Prop.ResourceType.Collection == nil {
		t.Error("dir must have collection resourcetype")
	}
	if rs.Propstat[0].Prop.GetContentLength != nil {
		t.Error("dir should not have content length")
	}
}

func TestMultistatusXML(t *testing.T) {
	fi := fakeInfo{name: "a.txt", size: 10, dir: false, mod: time.Unix(1700000000, 0)}
	ms := multistatus{XmlnsD: "DAV:"}
	ms.Responses = append(ms.Responses, makeResponse("/etc/a.txt", fi))

	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	if err := enc.Encode(ms); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "DAV:") {
		t.Errorf("missing xmlns: %s", out)
	}
	if !strings.Contains(out, "getcontentlength") {
		t.Errorf("missing getcontentlength: %s", out)
	}
	if !strings.Contains(out, "10") {
		t.Errorf("missing size: %s", out)
	}
}

func TestWithSlash(t *testing.T) {
	cases := map[string]string{
		"/":     "/",
		"":      "/",
		"/etc":  "/etc/",
		"/etc/": "/etc/",
	}
	for in, want := range cases {
		if got := withSlash(in); got != want {
			t.Errorf("withSlash(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeInfo implements os.FileInfo for tests.
type fakeInfo struct {
	name string
	size int64
	dir  bool
	mod  time.Time
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() os.FileMode  { return 0644 }
func (f fakeInfo) ModTime() time.Time { return f.mod }
func (f fakeInfo) IsDir() bool        { return f.dir }
func (f fakeInfo) Sys() interface{}   { return nil }

func TestParseRange(t *testing.T) {
	cases := []struct {
		rng   string
		size  int64
		start int64
		end   int64
		ok    bool
	}{
		{"bytes=0-99", 1000, 0, 99, true},
		{"bytes=100-", 1000, 100, 999, true},
		{"bytes=500-599", 1000, 500, 599, true},
		{"bytes=-100", 1000, 900, 999, true},
		{"bytes=-100", 50, 0, 49, true}, // suffix larger than file
		{"bytes=0-0", 1, 0, 0, true},
		{"bytes=2000-", 1000, 0, 0, false}, // start beyond size
		{"bytes=bad", 1000, 0, 0, false},
		{"", 1000, 0, 0, false},
		{"bytes=5-2", 1000, 0, 0, false}, // end < start
	}
	for _, c := range cases {
		s, e, ok := parseRange(c.rng, c.size)
		if ok != c.ok || (ok && (s != c.start || e != c.end)) {
			t.Errorf("parseRange(%q, %d) = (%d,%d,%v), want (%d,%d,%v)",
				c.rng, c.size, s, e, ok, c.start, c.end, c.ok)
		}
	}
}

func TestDestPath(t *testing.T) {
	cases := []struct {
		dest string
		want string
		ok   bool
	}{
		{"http://127.0.0.1:1234/tmp/foo", "/tmp/foo", true},
		{"http://127.0.0.1:1234/", "/", true},
		{"/tmp/foo", "/tmp/foo", true},
		{"", "", false},
	}
	for _, c := range cases {
		r := reqWithDest(c.dest)
		got, ok := destPath(r)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("destPath(%q) = (%q,%v), want (%q,%v)", c.dest, got, ok, c.want, c.ok)
		}
	}
}

func reqWithDest(dest string) *http.Request {
	r, _ := http.NewRequest("MOVE", "/src", nil)
	if dest != "" {
		r.Header.Set("Destination", dest)
	}
	return r
}
