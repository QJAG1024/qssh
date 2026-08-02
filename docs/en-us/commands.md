# Command Reference

## Profile Management

### `qssh --add`

Create a new profile. Runs interactively by default; non-interactive when
any of `--host` / `--user` / `--auth` is provided.

```bash
# Interactive
qssh --add myserver

# Non-interactive (agent-friendly)
qssh --add myserver --host 192.168.1.1 --user root --auth password --password "xxx"
qssh --add myserver --host example.com --user deploy --auth key --key-path ~/.ssh/id_ed25519
qssh --add myserver --host example.com --user deploy --auth agent
```

| Flag | Description |
| ------ | ------------- |
| `--host <host>` | SSH hostname or IP address |
| `--port <port>` | SSH port (default: `22`) |
| `--user <user>` | SSH username |
| `--auth <type>` | `password`, `key`, `agent`, `keyboard-interactive` |
| `--password <pass>` | Password (for `--auth password`) |
| `--key-path <path>` | Private key path (for `--auth key`) |
| `--key-passphrase <pass>` | Passphrase for encrypted private key |
| `--proxy <profile>` | Jump-host profile name |
| `--set-option KEY=VALUE,...` | Comma-separated SSH options |
| `--tags tag1,tag2,...` | Comma-separated tags |

### `qssh --edit`

Edit an existing profile. Same flags as `--add`.

```bash
qssh --edit myserver --host newhost.example.com
qssh --edit myserver --password newpass
qssh --edit myserver --proxy gateway
qssh --edit myserver --tags prod,web
```

### `qssh --delete`

Delete a profile. Confirms by default; skip with `--yes`.

```bash
qssh --delete myserver
qssh --delete myserver --yes
```

### `qssh --copy` / `qssh --rename`

```bash
qssh --copy myserver myserver-backup
qssh --rename myserver new-server
```

### `qssh --export` / `qssh --import`

Export/import profiles as **passphrase-encrypted** `.qssh` files (AES-256-GCM,
PBKDF2 key derivation). Secrets (password, key passphrase) are included;
private key files are embedded when present so cross-machine imports can
restore them. The profile **name is not exported** — the importer supplies it,
so there are no name collisions.

```bash
# Interactive (asks for passphrase + output dir; default: current dir)
qssh --export myserver

# Non-interactive (passphrase read from stdin)
printf 'passphrase\n' | qssh --export myserver --dir ~/backups

# Interactive import (asks for passphrase + profile name)
qssh --import ~/backups/myserver.qssh

# Non-interactive import (passphrase from stdin, explicit name)
printf 'passphrase\n' | qssh --import myserver.qssh --name myserver
```

Notes:

- Proxy (jump host) is **not** exported; a warning is printed at export time.
- Importing over an existing profile name is refused.
- If the exported key file path does not exist on the importing machine, the
  embedded key is restored to `~/.ssh/` and the profile's `key_path` is
  updated accordingly.

### `qssh --list`

List profiles. Optional substring filter. JSON output with `--json`.

```bash
qssh --list
qssh --list prod          # filter by name or host
qssh --list --json        # machine-readable (secrets redacted, hosts in privacy mode)
```

### Profile Options (`--set-option`)

| Key | Value | Description |
|-----|-------|-------------|
| `ConnectTimeout` | Duration (e.g. `30s`) | TCP+SSH handshake timeout. Default: `10s` |
| `SetEnv` | `KEY=VALUE,KEY2=VALUE2` | Environment variables on remote session |

```bash
qssh --add srv --host 10.0.0.1 --user root --auth password --password x \
     --set-option ConnectTimeout=30s,SetEnv=LANG=en_US.UTF-8
```

---

## Authentication Methods

| Method | CLI Flag | Requires |
| -------- | ---------- | ---------- |
| `password` | `--auth password` | `--password` |
| `key` | `--auth key` | `--key-path`; optional `--key-passphrase` |
| `agent` | `--auth agent` | `SSH_AUTH_SOCK` |
| `keyboard-interactive` | `--auth keyboard-interactive` | Interactive prompt |

---

## Connection

```bash
qssh <profile>
```

Opens an interactive shell with PTY, signal forwarding, and window resize.
Progress is shown for DNS resolution, TCP connect, SSH handshake, PTY
allocation, and shell start.

---

## Remote Command Execution

```bash
qssh --exec <profile> <command...>
```

### Behaviour

- **Hot path** (daemon running): completes in < 100 ms.
- **Cold start**: auto-forks a managed daemon, establishes SSH, runs the
  command. Daemon stays alive for 5 min idle.
- **Exit code**: propagated to the local process.
- **Stdin**: forwarded when piped (non-TTY). Binary-safe.
- **Args**: multiple arguments are individually shell-quoted; a single
  argument is treated as a full shell command for backward compatibility.

### Examples

```bash
# Simple command
qssh --exec srv "uptime"

# Multi-arg (safe quoting for spaces)
qssh --exec srv printf '%s\n' 'hello world'

# Pipe stdin
echo "data" | qssh --exec srv cat

# Binary stdin
tar czf - . | qssh --exec srv 'tar xzf - -C /tmp'
```

---

## SFTP Proxy

A local TCP server that proxies SFTP to the remote host. Any SFTP client
can connect.

```bash
# Start (random port)
qssh --sftp-start srv
# → SFTP proxy: sftp://127.0.0.1:33803

# Custom bind/port
qssh --sftp-start srv --bind 127.0.0.1 --port 22222

# Stop
qssh --sftp-stop srv
```

If a daemon is running, the SFTP proxy reuses its SSH connection.

**Bind authorization.** The proxy accepts any password (real auth is the
remote SSH connection), so the bind address is the security boundary. The
three ways to bind non-loopback:

| Source | Behavior |
| -------- | ---------- |
| `--bind 0.0.0.0` (CLI) | Warnings for 2s, then proceeds — you explicitly asked for it |
| profile `sftp.bind=0.0.0.0` | Allowed — the per-profile choice is the authorization |
| global `sftp.bind=0.0.0.0` | Refused unless `sftp.allow_non_loopback=true` is also set |

Setting a non-loopback global `sftp.bind` is not blocked at config time, but
`--sftp-start` will refuse to start it until `sftp.allow_non_loopback=true`.

`--sftp-allow-remote` is **deprecated** (kept for script compatibility):
non-loopback binds are now authorized by `--bind` or per-profile `sftp.bind`.

## WebDAV Mount

An SFTP-backed WebDAV server that mounts the remote file system in native
file managers (macOS Finder, Windows Explorer, Linux gvfs/KDE). Optimized
for high-latency links: PROPFIND builds the response from a single ReadDir
(no per-entry round-trips — see [webdav-design.md](webdav-design.md)).

```bash
# Start
qssh --webdav-start srv
# → WebDAV:
#     dav://127.0.0.1:34123/
#     http://127.0.0.1:34123/

# Stop
qssh --webdav-stop srv
```

Mount (per OS):
- **macOS**: Finder → Go → Connect to Server → `http://127.0.0.1:<port>/`
- **Windows**: Explorer → Map network drive → `http://127.0.0.1:<port>/`
- **Linux**: file manager address bar `dav://127.0.0.1:<port>/` (or `gio mount`)

**Token auth.** Loopback binds are open (same trust model as the SFTP proxy).
Non-loopback binds (`--bind 0.0.0.0` or per-profile `webdav.bind`) generate a
random token required via `X-QSSH-Token` header or `?token=` query; the
printed URL carries it. `webdav.token_mode=always` (global or per-profile)
requires a token even on loopback.


---

## Daemon (Connection Reuse)

| Mode | Trigger | Lifetime |
|------|---------|----------|
| **Managed** | Auto-started by `--exec` | Idle 5 min, auto-exit |
| **Persistent** | `qssh --daemon-start <profile>` | Until `--daemon-stop` |

```bash
qssh --daemon-start srv
qssh --daemon-stop srv
```

The daemon sends SSH keepalive probes every 30 s and auto-reconnects on
connection drop (unless SFTP is mounted).

---

<a id="jump-hosts"></a>

## Jump Hosts (Proxy Chains)

A profile can designate another profile as a jump host. Multi-hop chains
are supported.

```bash
# Create gateway
qssh --add gateway --host 192.168.1.1 --user root --auth key --key-path ~/.ssh/id_ed25519

# Create target behind gateway
qssh --add internal --host 10.0.0.5 --user root --auth password --password secret --proxy gateway

# Connect — tunnels through gateway automatically
qssh internal
qssh --exec internal hostname
```

Works with interactive connect, `--exec`, and SFTP.

---

## History

```bash
qssh --history               # all profiles
qssh --history myserver      # one profile
qssh --last                  # most recent entry
```

History is capped by `history.max_size` (default 5 MB). See
[config.md](config.md).

### Command recording

How much of each `--exec` command is stored in history is controlled by a
three-level mode. The safe default (`masked`) persists only the command name
(`docker compose up -d` → `docker`), so secrets passed as arguments never
reach disk.

| Mode | Behavior |
| ------ | ---------- |
| `full` | Persist the entire command line |
| `masked` (default) | Persist only the first token (command name) |
| `off` | Persist no command at all |

```bash
# Global default
qssh --config set history.record_commands full

# Per-profile override (wins over global)
qssh --add myserver --host 1.2.3.4 --set-option history.record_commands=off
qssh --edit myserver --set-option history.record_commands=masked
```

---

## Privacy

Host/IP addresses are redacted from UI output **by default**.

```bash
qssh --privacy               # show status
qssh --privacy off            # show hosts (sticky until reboot)
qssh --privacy on             # hide hosts (sticky)
qssh --privacy clear          # reset to default on
qssh --reveal --list           # show hosts for this process only
```

When enabled, the Host column is hidden in `--list`, JSON omits `host`,
and progress/error messages show `***` instead of addresses.

---

## Config

```bash
qssh --config                  # list all keys
qssh --config get <key>        # get value
qssh --config set <key> <val>  # set value
qssh --config unset <key>      # remove (revert to default)
```

Full config key reference: [config.md](config.md).

---

## Version

```bash
qssh --version
```
