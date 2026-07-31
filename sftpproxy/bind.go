package sftpproxy

import (
	"fmt"
	"net"
	"strings"
)

// IsLoopbackAddr reports whether addr (with optional port) resolves only to
// loopback addresses. Used to decide whether a bind requires authorization.
func IsLoopbackAddr(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false
		}
	}
	return true
}

// ValidateBindAddr rejects non-loopback bind addresses unless allowRemote
// is true. Empty host is treated as unspecified (0.0.0.0) and rejected.
// Hostnames that resolve only to loopback (e.g. "localhost") are allowed.
func ValidateBindAddr(bindAddr string, allowRemote bool) error {
	if allowRemote {
		return nil
	}
	host := bindAddr
	// Strip port if present (host:port or [ipv6]:port).
	if h, _, err := net.SplitHostPort(bindAddr); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("SFTP bind address is empty or unspecified; refusing non-loopback listen (set sftp.allow_non_loopback=true or pass --sftp-allow-remote to override)")
	}

	// Literal IPs.
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsLoopback() {
			return fmt.Errorf("SFTP bind address %q is not loopback; refusing remote listen (set sftp.allow_non_loopback=true or pass --sftp-allow-remote to override)", host)
		}
		return nil
	}

	// Hostnames: require every resolved address to be loopback.
	ips, err := net.LookupIP(host)
	if err != nil {
		// Unresolvable — fail closed rather than allowing a surprise bind.
		return fmt.Errorf("SFTP bind address %q cannot be resolved: %w (use 127.0.0.1 or pass --sftp-allow-remote)", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("SFTP bind address %q resolved to no addresses", host)
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return fmt.Errorf("SFTP bind address %q resolves to non-loopback %s; refusing remote listen (set sftp.allow_non_loopback=true or pass --sftp-allow-remote to override)", host, ip)
		}
	}
	return nil
}
