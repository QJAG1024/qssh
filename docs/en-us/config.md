# Configuration

All config is stored in `~/.config/qssh/config.json` as a flat key-value map.
Use `qssh --config` to manage settings.

## Config Keys

| Key | Values | Default | Description |
| ----- | -------- | --------- | ------------- |
| `store.backend` | `file`, `keyring` | Auto-probed | Master key storage backend. `file` uses `~/.config/qssh/store.key`; `keyring` uses GNOME Keyring (`secret-tool`). `file` is preferred when a `store.key` already exists. |
| `lang` | `en-US`, `zh-CN` | `en-US` | UI language. |
| `sftp.bind` | IP address | `127.0.0.1` | Default bind address for `--sftp-start`. Non-loopback requires `sftp.allow_non_loopback=true` (global) — or set it per-profile, where the profile choice itself authorizes the bind (see Per-profile overrides). |
| `webdav.bind` | IP address | `127.0.0.1` | Default bind address for `--webdav-start`. Non-loopback enables token auth automatically. Per-profile override via `--set-option webdav.bind=...`. |
| `webdav.token_mode` | `auto`, `always` | `auto` | WebDAV token auth: `auto` (loopback open, non-loopback token), `always` (token even on loopback). Per-profile override via `--set-option webdav.token_mode=...`. |
| `hostkey.mode` | `tofu`, `strict` | `tofu` | `tofu`: accept unknown host keys on first use (fingerprint logged to `hostkey.log`). `strict`: reject unknown hosts. |
| `term.mode` | `passthrough`, `compat` | `passthrough` | `passthrough`: send local `$TERM` as-is to the remote PTY. `compat`: force `xterm` (for hosts missing `ncurses-term`). |
| `history.max_size` | Size string | `5M` | Maximum history file size. Supports `K`/`M`/`G` suffixes. Oldest entries are trimmed on each append. |
| `history.record_commands` | `full`, `masked`, `off` | `masked` | Default for how much of each `--exec` command is persisted to history. `full`: entire command line. `masked`: command name only (first token). `off`: no command. Per-profile override via `--set-option history.record_commands=...` (same key name; profile wins) — see [commands.md](commands.md). |

### Size String Format

`history.max_size` accepts:

| Format | Example | Meaning |
| -------- | --------- | --------- |
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

## Per-profile overrides

Keys marked per-profile can be overridden on a single profile with
`--set-option key=value` (interactive Options edit too). Same key name as the
global config, profile wins. An empty value (`--set-option key=`) clears the
override and falls back to the global default.

Supported per-profile keys: `ConnectTimeout`, `SetEnv`, `term.mode`,
`hostkey.mode`, `history.record_commands`, `sftp.bind`.

```bash
qssh --edit myserver --set-option term.mode=compat
qssh --edit myserver --set-option hostkey.mode=strict
qssh --edit myserver --set-option history.record_commands=off
qssh --edit myserver --set-option sftp.bind=0.0.0.0
qssh --edit myserver --set-option term.mode=      # clear override
```

`SetEnv` merges per-variable: `--set-option SetEnv=FOO=bar` adds/updates just
`FOO`; `--set-option SetEnv=FOO=` removes it. Other keys replace wholesale.

`sftp.allow_non_loopback` is **global-only**; to bind a profile's SFTP proxy
non-loopback, set `sftp.bind` on the profile (the explicit per-profile choice
is the authorization — no separate allow flag needed). For a global
non-loopback default you must set both `sftp.bind` and
`sftp.allow_non_loopback=true`.

---

<a id="environment-variables"></a>

## Environment Variables

| Variable | Default | Description |
| ---------- | --------- | ------------- |
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
