package sftpproxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// --- SFTP handler — forwards every call to the remote sftp.Client ---

type sftpProxy struct {
	remote *sftp.Client
}

// compile-time checks
var (
	_ sftp.FileReader           = (*sftpProxy)(nil)
	_ sftp.FileWriter           = (*sftpProxy)(nil)
	_ sftp.OpenFileWriter       = (*sftpProxy)(nil)
	_ sftp.FileCmder            = (*sftpProxy)(nil)
	_ sftp.FileLister           = (*sftpProxy)(nil)
	_ sftp.LstatFileLister      = (*sftpProxy)(nil)
	_ sftp.ReadlinkFileLister   = (*sftpProxy)(nil)
	_ sftp.RealPathFileLister   = (*sftpProxy)(nil)
)

func (p *sftpProxy) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	f, err := p.remote.OpenFile(r.Filepath, os.O_RDONLY)
	return f, err
}

func (p *sftpProxy) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	f, err := p.remote.OpenFile(r.Filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	return f, err
}

func (p *sftpProxy) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	f, err := p.remote.OpenFile(r.Filepath, os.O_RDWR|os.O_CREATE)
	return f, err
}

func (p *sftpProxy) Filecmd(r *sftp.Request) error {
	switch r.Method {
	case "Mkdir":
		return p.remote.MkdirAll(r.Filepath)
	case "Rmdir":
		return p.remote.RemoveDirectory(r.Filepath)
	case "Remove":
		return p.remote.Remove(r.Filepath)
	case "Rename":
		return p.remote.Rename(r.Filepath, r.Target)
	case "Symlink":
		return p.remote.Symlink(r.Filepath, r.Target)
	case "Setstat":
		return p.handleSetstat(r)
	default:
		return fmt.Errorf("unsupported Filecmd method: %s", r.Method)
	}
}

func (p *sftpProxy) handleSetstat(r *sftp.Request) error {
	attrs := &sshFxpAttributes{}
	if err := attrs.fromBytes(r.Attrs); err != nil {
		return err
	}
	if attrs.HasSize {
		if err := p.remote.Truncate(r.Filepath, int64(attrs.Size)); err != nil {
			return err
		}
	}
	if attrs.HasPermissions {
		if err := p.remote.Chmod(r.Filepath, attrs.FileMode()); err != nil {
			return err
		}
	}
	if attrs.HasUIDGID {
		if err := p.remote.Chown(r.Filepath, int(attrs.UID), int(attrs.GID)); err != nil {
			return err
		}
	}
	if attrs.HasModTime {
		if err := p.remote.Chtimes(r.Filepath, time.Now(), attrs.ModTime); err != nil {
			return err
		}
	}
	return nil
}

func (p *sftpProxy) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	switch r.Method {
	case "Stat":
		fi, err := p.remote.Stat(r.Filepath)
		if err != nil {
			return nil, err
		}
		return &singleFileInfo{fi}, nil
	case "List":
		entries, err := p.remote.ReadDir(r.Filepath)
		if err != nil {
			return nil, err
		}
		return &dirEntries{fi: entries}, nil
	default:
		return nil, fmt.Errorf("unexpected Filelist method: %s", r.Method)
	}
}

func (p *sftpProxy) Lstat(r *sftp.Request) (sftp.ListerAt, error) {
	fi, err := p.remote.Lstat(r.Filepath)
	if err != nil {
		return nil, err
	}
	return &singleFileInfo{fi}, nil
}

func (p *sftpProxy) Readlink(path string) (string, error) {
	return p.remote.ReadLink(path)
}

func (p *sftpProxy) RealPath(path string) (string, error) {
	return p.remote.RealPath(path)
}

// --- ListerAt implementations ---

type singleFileInfo struct {
	os.FileInfo
}

func (s *singleFileInfo) ListAt(fi []os.FileInfo, offset int64) (int, error) {
	if offset > 0 {
		return 0, io.EOF
	}
	if len(fi) > 0 {
		fi[0] = s.FileInfo
		return 1, io.EOF
	}
	return 0, nil
}

type dirEntries struct {
	fi []os.FileInfo
}

func (d *dirEntries) ListAt(fi []os.FileInfo, offset int64) (int, error) {
	if int(offset) >= len(d.fi) {
		return 0, io.EOF
	}
	n := copy(fi, d.fi[offset:])
	if offset+int64(n) >= int64(len(d.fi)) {
		return n, io.EOF
	}
	return n, nil
}

// --- SSH + SFTP proxy serve loop ---

func sftpProxyHandlers(remote *sftp.Client) sftp.Handlers {
	p := &sftpProxy{remote: remote}
	return sftp.Handlers{
		FileGet:  p,
		FilePut:  p,
		FileCmd:  p,
		FileList: p,
	}
}

// handleProxyConn upgrades the TCP connection to SSH, then serves SFTP over it.
func handleProxyConn(remote *sftp.Client, conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			newCh.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		ch, reqs, err := newCh.Accept()
		if err != nil {
			continue
		}

		go func() {
			defer ch.Close()
			for req := range reqs {
				if req.Type != "subsystem" {
					req.Reply(false, nil)
					continue
				}
				name := parseSubsystemName(req.Payload)
				if name != "sftp" {
					req.Reply(false, nil)
					continue
				}
				req.Reply(true, nil)

				handlers := sftpProxyHandlers(remote)
				server := sftp.NewRequestServer(ch, handlers)
				server.Serve()
				return
			}
		}()
	}
}

// parseSubsystemName extracts the subsystem name from an SSH "subsystem"
// request payload (SSH wire string: uint32 length + UTF-8 data).
func parseSubsystemName(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	l := binary.BigEndian.Uint32(payload[:4])
	if int(l) != len(payload)-4 {
		return ""
	}
	return string(payload[4:])
}

// --- sshFxpAttributes: minimal parser for Setstat payload ---

type sshFxpAttributes struct {
	HasSize        bool
	HasUIDGID      bool
	HasPermissions bool
	HasModTime     bool
	Size           uint64
	UID, GID       uint32
	Permissions    uint32
	ModTime        time.Time
}

func (a *sshFxpAttributes) FileMode() os.FileMode {
	return os.FileMode(a.Permissions) & os.ModePerm
}

func (a *sshFxpAttributes) fromBytes(b []byte) error {
	if len(b) < 4 {
		return nil
	}
	flags := binary.BigEndian.Uint32(b[:4])
	off := 4

	if flags&sshFileXferAttrSize != 0 {
		if off+8 > len(b) {
			return io.ErrUnexpectedEOF
		}
		a.HasSize = true
		a.Size = binary.BigEndian.Uint64(b[off:])
		off += 8
	}
	if flags&sshFileXferAttrUIDGID != 0 {
		if off+8 > len(b) {
			return io.ErrUnexpectedEOF
		}
		a.HasUIDGID = true
		a.UID = binary.BigEndian.Uint32(b[off:])
		a.GID = binary.BigEndian.Uint32(b[off+4:])
		off += 8
	}
	if flags&sshFileXferAttrPermissions != 0 {
		if off+4 > len(b) {
			return io.ErrUnexpectedEOF
		}
		a.HasPermissions = true
		a.Permissions = binary.BigEndian.Uint32(b[off:])
		off += 4
	}
	if flags&sshFileXferAttrACmodTime != 0 {
		if off+8 > len(b) {
			return io.ErrUnexpectedEOF
		}
		a.HasModTime = true
		a.ModTime = time.Unix(int64(binary.BigEndian.Uint32(b[off:])), 0)
		off += 8
	}
	return nil
}

// SFTP attribute flags from the spec
const (
	sshFileXferAttrSize        uint32 = 0x00000001
	sshFileXferAttrUIDGID      uint32 = 0x00000002
	sshFileXferAttrPermissions uint32 = 0x00000004
	sshFileXferAttrACmodTime   uint32 = 0x00000008
)

// --- Public entry point ---

// StartSFTPServer starts a local SSH server on listener that accepts any
// password and serves an SFTP subsystem that proxies to remote.
func StartSFTPServer(remote *sftp.Client, listener net.Listener, signer ssh.Signer) error {
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil // accept any password
		},
	}
	config.AddHostKey(signer)

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go handleProxyConn(remote, conn, config)
	}
}

// LoadHostKey generates/loads the persistent SSH host key and returns it.
func LoadHostKey(cfgDir string) (ssh.Signer, error) {
	return loadOrGenerateHostKey(cfgDir)
}

// GetFingerprint returns the public key fingerprint for the persistent host key.
func GetFingerprint(cfgDir string) (string, error) {
	signer, err := loadOrGenerateHostKey(cfgDir)
	if err != nil {
		return "", err
	}
	return ssh.FingerprintSHA256(signer.PublicKey()), nil
}

// --- Host key management ---

// hostKeyFile is the path where the SSH host key is cached.
const hostKeyFile = "sftp_host_key"

func loadOrGenerateHostKey(configDir string) (ssh.Signer, error) {
	keyPath := filepath.Join(configDir, hostKeyFile)

	// Try to load existing key.
	data, err := os.ReadFile(keyPath)
	if err == nil {
		signer, err := ssh.ParsePrivateKey(data)
		if err == nil {
			return signer, nil
		}
	}

	// Generate new key.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	// Cache to disk for next time (PEM-encoded).
	privBytes := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err == nil {
		os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600)
	}

	return signer, nil
}