# AI Agent Guide

QSSH is designed for agent-driven workflows. All operations can be
performed non-interactively.

## Core Patterns

### List and filter

```bash
# Machine-readable JSON (secrets redacted, hosts hidden in privacy mode)
qssh --list --json

# Filter by profile name or host
qssh --list --json | jq '.[] | select(.name | test("prod"))'
```

### Execute commands

```bash
# Simple command
qssh --exec srv "systemctl status nginx"

# Multi-arg with spaces
qssh --exec srv printf '%s\n' 'hello world'

# Pipe data through
echo "SELECT 1" | qssh --exec db "mysql -e '$(cat)'"

# Binary data
tar czf - /data | qssh --exec backup "cat > /backup/data.tgz"
```

### Create profiles

```bash
qssh --add srv --host 10.0.0.1 --port 22 --user root --auth password --password "xxx"
qssh --add srv --host 10.0.0.1 --user root --auth key --key-path ~/.ssh/id_ed25519
qssh --add srv --host 10.0.0.1 --user root --auth key --key-path ~/.ssh/id_ed25519 --key-passphrase "secret"
qssh --add behind-gw --host 10.0.0.5 --user root --auth password --password x --proxy gateway
qssh --add srv --host 10.0.0.1 --user root --auth password --password x --tags prod,web
```

### Delete without prompt

```bash
qssh --delete old-server --yes
qssh --delete old-server -y
```

### Debug with revealed hosts

```bash
qssh --reveal --list --json   # one-shot: show IPs in this process
qssh --privacy off             # sticky: show IPs until reboot
```

### Check remote system status

```bash
qssh --exec srv "uptime && free -m && df -h /"
qssh --exec srv "systemctl is-active nginx docker"
```

### Deploy software

```bash
qssh --exec srv "cd /app && git pull && systemctl restart app"
```

### SFTP proxy for file transfer

```bash
qssh --sftp-start srv
# → SFTP proxy: sftp://127.0.0.1:33803
# Then use any SFTP client to transfer files

qssh --sftp-stop srv
```

---

## Design Properties for Agents

- **No interactive prompts**: `--exec` auto-starts a managed daemon.
  `--delete --yes` skips confirmation. `--add` with flags runs
  non-interactively.
- **Exit codes**: `--exec` propagates the remote exit code. The agent can
  branch on success/failure.
- **Stdin forwarding**: pipe data directly to remote commands.
- **Binary-safe**: base64-encoded frames in the daemon protocol.
- **Connection reuse**: after the first `--exec`, the daemon keeps the SSH
  connection alive. Subsequent calls complete in < 100 ms.
- **Idle cleanup**: managed daemons exit after 5 minutes of inactivity.
- **Privacy**: hosts are hidden by default in list output and error messages.
  Use `--reveal` for debugging.
- **JSON output**: `--list --json` produces parseable output with secrets
  redacted.

---

## Tips

### Use profile names as stable identifiers

Profile names are the primary identity. Agents should reference profiles
by name, not by IP — the host is hidden in privacy mode and may change.

### Prefer `--exec` over interactive shell

`qssh --exec srv "cmd"` is the right tool for agents. `qssh srv` opens an
interactive PTY shell — only use it when a human is at the keyboard.

### Chain commands with pipes

```bash
# Run a sequence
qssh --exec srv "cd /app && git pull && go build && systemctl restart app"

# Or use stdin for complex scripts
cat deploy.sh | qssh --exec srv bash
```

### Manage daemon lifecycle for long sessions

If you need persistent connections across many commands:

```bash
qssh --daemon-start srv      # start persistent daemon
# ... many --exec calls ...
qssh --daemon-stop srv       # stop when done
```

Otherwise, let the managed daemon handle it automatically.