# Configuration

All config is stored in `~/.config/qssh/config.json` as a flat key-value map.
Use `qssh --config` to manage settings.

## Config Keys

| Key | Values | Default | Description |
|-----|--------|---------|-------------|
| `store.backend` | `file`, `keyring` | Auto-probed | Master key storage backend. `file` uses `~/.config/qssh/store.key`; `keyring` uses GNOME Keyring (`secret-tool`). `file` is preferred when a `store.key` already exists. |
| `lang` | `en-US`, `zh-CN` | `en-US` | UI language. |
| `sftp.bind` | IP address | `127.0.0.1` | Default bind address for `--sftp-start`. |
| `hostkey.mode` | `tofu`, `strict` | `tofu` | `tofu`: accept unknown host keys on first use (fingerprint logged to `hostkey.log`). `strict`: reject unknown hosts. |
| `term.mode` | `passthrough`, `compat` | `passthrough` | `passthrough`: send local `$TERM` as-is to the remote PTY. `compat`: force `xterm` (for hosts missing `ncurses-term`). |
| `history.max_size` | Size string | `5M` | Maximum history file size. Supports `K`/`M`/`G` suffixes. Oldest entries are trimmed on each append. |

### Size String Format

`history.max_size` accepts:

| Format | Example | Meaning |
|--------|---------|---------|
| Raw bytes | `1048576` | 1,048,576 bytes |
| K suffix | `500K` | 500 × 1024 = 512,000 bytes |
| M suffix | `5M` | 5 × 1024² = 5,242,880 bytes |
| G suffix | `1G` | 1 × 1024³ bytes |

## Examples

```bash
qssh --config set lang zh-CN
qssh --config set hostkey.mode strict
qssh --config set history.max_size 10M
qssh --config set term.mode compat
qssh --config unset term.mode          # revert to passthrough
qssh --config get lang                 # prints "zh-CN"
```

---

<a id="environment-variables"></a>
## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `QSSH_STORE_PATH` | `~/.config/qssh/store.json` | Override store file path |
| `QSSH_KEY_PATH` | `~/.config/qssh/store.key` | Override key file path |
| `QSSH_HISTORY_PATH` | `~/.config/qssh/history.jsonl` | Override history file path |
| `QSSH_PRIVACY` | _(unset)_ | Force privacy mode: `on`, `off` (overrides sticky, not config) |
| `QSSH_KNOWN_HOSTS` | `~/.config/qssh/known_hosts` | Override known_hosts file path |
| `XDG_RUNTIME_DIR` | OS default | Runtime directory for sticky privacy state |
| `SSH_AUTH_SOCK` | _(agent)_ | SSH agent socket (for `--auth agent`) |

---

## Sticky Privacy State

`--privacy on|off` writes to `$XDG_RUNTIME_DIR/qssh/privacy` (mode `0600`).
This file is automatically cleared on reboot. It is **not** a config key —
it is runtime state separate from `config.json`.