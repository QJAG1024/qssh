package webdav

import (
	"bytes"
	"encoding/xml"
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
