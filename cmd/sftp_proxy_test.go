package cmd

import (
	"path/filepath"
	"testing"

	"qssh/internal"
)

// isolateConfig points DefaultConfigPath at a temp file for tests that read
// global config (sftp.bind / sftp.allow_non_loopback).
func isolateConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "qssh", "config.json")
	t.Setenv("QSSH_CONFIG_PATH", cfgPath)
}

func TestResolveSFTPBind(t *testing.T) {
	isolateConfig(t)

	// 1. CLI --bind wins, non-loopback authorized.
	addr, allow, origin := resolveSFTPBind("0.0.0.0", nil)
	if addr != "0.0.0.0" || !allow || origin != bindOriginCLI {
		t.Errorf("CLI non-loopback: got (%s, %v, %s)", addr, allow, origin)
	}
	addr, allow, origin = resolveSFTPBind("127.0.0.1", nil)
	if addr != "127.0.0.1" || allow || origin != bindOriginCLI {
		t.Errorf("CLI loopback: got (%s, %v, %s)", addr, allow, origin)
	}

	// 2. Profile sftp.bind authorizes non-loopback.
	addr, allow, origin = resolveSFTPBind("", map[string]string{"sftp.bind": "0.0.0.0"})
	if addr != "0.0.0.0" || !allow || origin != bindOriginProfile {
		t.Errorf("profile non-loopback: got (%s, %v, %s)", addr, allow, origin)
	}

	// 3. Global sftp.bind without allow -> not authorized (refused upstream).
	if err := internal.OpenConfig(internal.DefaultConfigPath()).Set("sftp.bind", "0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	addr, allow, origin = resolveSFTPBind("", nil)
	if addr != "0.0.0.0" || allow || origin != bindOriginGlobal {
		t.Errorf("global no-allow: got (%s, %v, %s), want (0.0.0.0, false, global)", addr, allow, origin)
	}

	// 4. Global sftp.bind + allow_non_loopback=true -> authorized.
	if err := internal.OpenConfig(internal.DefaultConfigPath()).Set("sftp.allow_non_loopback", "true"); err != nil {
		t.Fatal(err)
	}
	addr, allow, origin = resolveSFTPBind("", nil)
	if addr != "0.0.0.0" || !allow || origin != bindOriginGlobal {
		t.Errorf("global with-allow: got (%s, %v, %s)", addr, allow, origin)
	}

	// 5. Global loopback -> never needs allow, not authorized flag.
	if err := internal.OpenConfig(internal.DefaultConfigPath()).Set("sftp.bind", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	addr, allow, origin = resolveSFTPBind("", nil)
	if addr != "127.0.0.1" || allow || origin != bindOriginGlobal {
		t.Errorf("global loopback: got (%s, %v, %s)", addr, allow, origin)
	}

	// 6. Nothing set -> default loopback.
	if err := internal.OpenConfig(internal.DefaultConfigPath()).Set("sftp.bind", ""); err != nil {
		t.Fatal(err)
	}
	addr, allow, origin = resolveSFTPBind("", nil)
	if addr != "127.0.0.1" || allow || origin != bindOriginDefault {
		t.Errorf("default: got (%s, %v, %s)", addr, allow, origin)
	}
}
