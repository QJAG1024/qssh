---
name: qssh
description: "QSSH SSH credential manager for CLI and AI agent usage. Execute remote commands, proxy SFTP mounts, and reuse SSH connections via managed daemon. Use when the user wants to run a command on a remote host, check system status, collect logs, deploy software, start/stop an SFTP proxy, or manage remote servers from the terminal. Triggers on: 'qssh', '--exec', '--sftp', '--daemon', 'remote command', 'SSH exec', 'run on server', 'deploy to host', 'check remote host'."
license: MIT
metadata:
  version: "1.0"
  category: developer-tools
---

# QSSH Skill

QSSH is an SSH credential manager for CLI and AI agent usage. It stores credentials encrypted, connects to remote hosts, executes commands, and proxies SFTP access — all via single commands. No interactive prompts needed.

## Quick Start

```bash
# List available hosts (profiles)
./qssh --list

# Execute a command on a remote host
./qssh --exec <host> "<command>"
```

> **Path note**: If `qssh` is not in `$PATH`, use `./qssh` from the project directory or the absolute path.

## Core Behavior: Managed Daemon

`--exec` auto-starts a **managed daemon** that reuses the SSH connection across calls. This is the key design: you don't need to worry about connection lifecycle.

1. **First call** — forks a background daemon, establishes SSH connection, runs the command. The daemon stays alive afterward.
2. **Subsequent calls** — reuse the daemon's SSH connection. Near-instant (~0.1s).
3. **Idle timeout** — daemon auto-exits after 5 minutes of no connections. No manual cleanup needed.

This means you can chain `--exec` calls freely without performance penalty or connection overhead:

```bash
./qssh --exec myserver "uptime"
./qssh --exec myserver "df -h /"
./qssh --exec myserver "free -m"
```

The second and third calls complete in <100ms. No need to manage any daemon state yourself.

## Command Reference

| Command | Description |
|---------|-------------|
| `--exec <host> "<cmd>"` | Execute a command. Returns stdout + exit code. |
| `--sftp-start <host>` | Start SFTP proxy (returns port + fingerprint). |
| `--sftp-stop <host>` | Stop running SFTP proxy. |
| `--daemon-start <host>` | Start persistent daemon (long-lived, manual stop). |
| `--daemon-stop <host>` | Stop persistent daemon. |
| `--list [filter]` | List profiles, optional substring filter. |
| `--add <name> [flags]` | Add a profile (interactive, or with flags below). |

## Common Patterns

### Single probe — one-shot check

```bash
./qssh --exec myserver "systemctl is-active sshd"
```

Exit code 0 = active, non-zero = inactive.

### Multiple information gathering — fast sequential

```bash
./qssh --exec myserver "cat /etc/os-release"
./qssh --exec myserver "uname -r"
./qssh --exec myserver "uptime -p"
./qssh --exec myserver "lscpu | grep 'Model name'"
```

All share the same SSH connection. No repeated authentication.

### Deploy / update workflow

```bash
./qssh --exec myserver "cd /opt/app && git pull && systemctl restart app"
./qssh --exec myserver "systemctl status app --no-pager"
```

Batch related work into one command when possible, use separate calls when you need to react to intermediate output.

### SFTP file transfer

```bash
# Start proxy (daemon auto-reuses connection if running)
./qssh --sftp-start myserver
# → port=43210 fingerprint=SHA256:abc...

# Use any SFTP client to connect at localhost:43210
# (any password works — it's transparent)

# Stop proxy
./qssh --sftp-stop myserver
```

### Profile creation — single-line (non-interactive)

Create profiles without interactive prompts. All fields optional — missing ones default or prompt.

```bash
# Password auth
./qssh --add myserver --host 192.168.1.1 --user root --auth password --password "mypass"

# Key auth
./qssh --add myserver --host 192.168.1.1 --user root --auth key --key-path ~/.ssh/id_ed25519

# Agent auth
./qssh --add myserver --host example.com --user deploy --auth agent

# Port override (default: 22)
./qssh --add myserver --host 10.0.0.1 --port 2222 --user admin --auth password --password "..."
```

> **Security note**: `--password` exposes the password in the process list. For shared environments, omit it — the tool will prompt securely.

### Persistent daemon (for long-running monitoring)

```bash
./qssh --daemon-start myserver
# Daemon stays alive until explicitly stopped.
./qssh --daemon-stop myserver
```

In most cases you don't need this — `--exec` auto-starts a managed daemon automatically.

## AI Agent Rules

1. **DO** chain multiple `--exec` calls freely. The managed daemon handles connection reuse.
2. **DO** batch operations into one command when possible (e.g. `"cd /app && git pull && make build"`) for atomicity.
3. **DO** check exit code / error to determine if a remote command succeeded.
4. **DO** use `--add` with `--host`/`--user`/`--auth` flags to create profiles non-interactively.
5. **DO NOT** manually `--daemon-start` before `--exec`. Let the managed daemon auto-start.
6. **DO NOT** try to read credentials or SSH keys directly. Use `--exec` for remote access.
7. **DO NOT** run interactive commands (no PTY allocation). If the command needs user input, it will hang.
8. **DO NOT** use shell-specific syntax unless the remote host supports it. Prefer POSIX-compatible commands.

## Notes

- First `--exec` call takes 1-3s (SSH connection setup). Subsequent calls are ~0.1s.
- Credentials can be added non-interactively via `--add <name> --host <h> --user <u> --auth <method>` flags.
- SFTP proxy accepts any password (transparent forwarding to the remote host).
- Supported auth methods: password (encrypted), SSH key (with or without passphrase), SSH Agent, keyboard-interactive.
