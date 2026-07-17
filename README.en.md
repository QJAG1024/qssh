<div align="center">

<h1>QSSH</h1>
<h3>Simple & Fast SSH Credential Manager in the Terminal</h3>

[中文](./README.md) | English

</div>

```
qjag186@QJAG-Legion-EOS ~> ./qssh homelab
Profile: homelab (root@192.168.10.139:22)
  ✔ Profile loaded
  → Resolving 192.168.10.139
  ✔ DNS Resolution (192.168.10.139 → 192.168.10.139 (0ms))
  → Connecting to 192.168.10.139:22
  ✔ SSH Handshake (Connected in 26ms)
  → PTY Allocation
  ✔ PTY Allocation
  → Start Shell
  ✔ Session established, entering interactive mode
```

No need to open a dedicated desktop SSH client or re-enter passwords — just one command in your favorite terminal emulator.

## Installation

```bash
go build -o qssh .
```

## Usage

### Adding Credentials

```bash
# Interactive
./qssh --add myserver

# One-liner (AI agent-friendly)
./qssh --add myserver --host 192.168.1.1 --user root --auth password --password "xxx"
./qssh --add myserver --host 192.168.1.1 --user root --auth key --key-path ~/.ssh/id_ed25519
./qssh --add myserver --host example.com --user deploy --auth agent
```

Four authentication methods:

| Method | Description |
|---|---|
| `password` | Password auth, encrypted storage |
| `key` | Private key (optional passphrase) |
| `agent` | SSH Agent (SSH_AUTH_SOCK) |
| `keyboard-interactive` | Interactive prompts (e.g. 2FA) |

### Connecting

```bash
./qssh myserver
```

Step-by-step progress: DNS resolution, TCP connection, SSH handshake, authentication, PTY allocation, shell start.

### Remote Command Execution

Execute a command on a remote host and return the exit code.

```bash
./qssh --exec myserver "uptime"
./qssh --exec myserver "uname -a"
./qssh --exec myserver "systemctl status sshd"
```

The first call auto-starts a managed daemon for connection reuse. Subsequent calls return instantly — no re-authentication. The daemon auto-exits after 5 minutes of idle.

Built for AI agents and scripts — just call `--exec`, the rest is handled for you.

### Remote File Access (SFTP Proxy)

A local transparent SFTP proxy. Any SFTP client can connect.

```bash
./qssh --sftp-start myserver
./qssh --sftp-stop myserver
```

### Daemon (Connection Reuse)

| Mode | Description |
|---|---|
| `managed` | Auto-started by `--exec`, idle 5 min auto-exit |
| `persistent` | Manual `--daemon-start` / `--daemon-stop` |

### More Features

- **Jump hosts**: configure `--proxy` for automatic multi-hop tunneling
- **Privacy mode**: host/IP addresses are redacted in UI output by default; use `--reveal` to show them temporarily
- **Agent-friendly**: `--yes` to skip prompts, `--list --json` for machine-readable output, `--exec` with stdin piping
- **Host keys**: TOFU (accept on first use) with fingerprint audit log

## Full Documentation

All commands, config keys, and architecture details: [docs](docs/en-us/README.md).

## Data Storage

- Credentials: `~/.config/qssh/store.json` (AES-256-GCM encrypted)
- Master key: `~/.config/qssh/store.key` or GNOME Keyring (`secret-tool`)
- Known hosts: `~/.config/qssh/known_hosts`
- Daemon: `~/.config/qssh/<profile>.sock` (Unix socket)
- Config: `~/.config/qssh/config.json`

## Dependencies

- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) — SSH protocol + host key verification
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — terminal raw mode
- [github.com/pkg/sftp](https://github.com/pkg/sftp) — SFTP client & proxy