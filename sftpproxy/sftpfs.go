package sftpproxy

import (
	"context"
	"io/fs"
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/net/webdav"
)

// sftpFS implements webdav.FileSystem backed by an SFTP client.
type sftpFS struct {
	client *sftp.Client
}

// compile-time check
var _ webdav.FileSystem = (*sftpFS)(nil)

func (fs *sftpFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return fs.client.MkdirAll(name)
}

func (fs *sftpFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	// Map Go file flags to SFTP pflags.
	// sftp.OpenFile accepts the same flags as os.OpenFile.
	sf, err := fs.client.OpenFile(name, flag)
	if err != nil {
		return nil, err
	}
	return &sftpFile{File: sf, client: fs.client, name: name}, nil
}

func (fs *sftpFS) RemoveAll(ctx context.Context, name string) error {
	return fs.client.RemoveAll(name)
}

func (fs *sftpFS) Rename(ctx context.Context, oldName, newName string) error {
	return fs.client.Rename(oldName, newName)
}

func (fs *sftpFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	return fs.client.Stat(name)
}

// sftpFile wraps sftp.File to implement webdav.File (adds Readdir).
type sftpFile struct {
	*sftp.File
	client *sftp.Client
	name   string
}

// compile-time check
var _ webdav.File = (*sftpFile)(nil)

func (f *sftpFile) Readdir(count int) ([]fs.FileInfo, error) {
	// sftp.Client.ReadDir returns all entries; count semantics are best-effort.
	entries, err := f.client.ReadDir(f.name)
	if err != nil {
		return nil, err
	}
	if count > 0 && len(entries) > count {
		entries = entries[:count]
	}
	return entries, nil
}