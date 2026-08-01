package store

import (
	"strings"
	"testing"
)

func TestExportImportRoundtrip(t *testing.T) {
	p := Profile{
		Name:     "ignored", // name must NOT be part of the payload
		Host:     "example.com",
		Port:     2222,
		User:     "alice",
		Auth:     AuthPassword,
		Password: "s3cret",
		Proxy:    "jump", // should be flagged as dropped
	}
	data, err := ExportProfile(p, "hunter2", nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !IsExportFile(data) {
		t.Fatal("output should be recognized as export file")
	}
	payload, err := ImportProfile(data, "hunter2")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if payload.Host != "example.com" || payload.User != "alice" || payload.Port != 2222 {
		t.Errorf("payload mismatch: %+v", payload)
	}
	if payload.Password != "s3cret" {
		t.Error("password should be exported")
	}
	if !payload.ProxyDropped {
		t.Error("proxy should be flagged as dropped")
	}
}

func TestExportStripsName(t *testing.T) {
	p := Profile{Name: "secret-name", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}
	data, _ := ExportProfile(p, "pw", nil)
	payload, err := ImportProfile(data, "pw")
	if err != nil {
		t.Fatal(err)
	}
	// Name is not in exportPayload at all.
	if payload.Host != "h" {
		t.Errorf("payload = %+v", payload)
	}
}

func TestImportWrongPassphrase(t *testing.T) {
	p := Profile{Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}
	data, _ := ExportProfile(p, "correct", nil)
	_, err := ImportProfile(data, "wrong")
	if err == nil {
		t.Fatal("wrong passphrase should fail")
	}
	if !strings.Contains(err.Error(), "wrong passphrase") {
		t.Errorf("want wrong-passphrase error, got %v", err)
	}
}

func TestImportNotExportFile(t *testing.T) {
	_, err := ImportProfile([]byte("this is just a text file"), "pw")
	if err == nil || !strings.Contains(err.Error(), "not a qssh export") {
		t.Errorf("want not-export error, got %v", err)
	}
}

func TestImportUnsupportedVersion(t *testing.T) {
	// Hand-craft a future-version header: valid magic, version 9, garbage body.
	data := []byte("QSSHX9:AAAA")
	_, err := ImportProfile(data, "pw")
	if err == nil || !strings.Contains(err.Error(), "unsupported export version 9") {
		t.Errorf("want version error, got %v", err)
	}
}

func TestExportKeyDataEmbedded(t *testing.T) {
	p := Profile{Host: "h", Port: 22, User: "u", Auth: AuthKey, KeyPath: "/home/u/.ssh/id_rsa"}
	keyContent := []byte("-----BEGIN RSA PRIVATE KEY-----\n...\n")
	data, _ := ExportProfile(p, "pw", keyContent)
	payload, err := ImportProfile(data, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if string(payload.KeyData) != string(keyContent) {
		t.Error("key data should roundtrip")
	}
	if payload.KeyPath != "/home/u/.ssh/id_rsa" {
		t.Errorf("key path = %q", payload.KeyPath)
	}
}

func TestExportSaltRandomness(t *testing.T) {
	p := Profile{Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}
	d1, _ := ExportProfile(p, "pw", nil)
	d2, _ := ExportProfile(p, "pw", nil)
	if string(d1) == string(d2) {
		t.Error("same passphrase+profile should still produce different files (random salt)")
	}
}
