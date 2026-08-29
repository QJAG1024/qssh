# Architecture

## Data Flow

```text
┌──────────┐     ┌──────────┐     ┌──────────┐
│  store.json │───▶│  load()   │───▶│  profiles │
│  (encrypted)│    │  decrypt  │    │  (memory) │
└──────────┘     └──────────┘     └──────────┘
       ▲               │
       │         ┌─────▼──────┐
  ┌────┴────┐    │  keyring    │
  │ store.key│    │ secret-tool│
  └─────────┘    └────────────┘

┌──────────┐     ┌──────────┐     ┌──────────┐
│  qssh     │────▶│  daemon  │────▶│  SSH     │
│  --exec   │     │  (socket)│     │  (keepalive
│  client   │◀────│          │◀────│  +reconnect)
└──────────┘     └──────────┘     └──────────┘
```

### Key Packages

| Package | Role |
| --------- | ------ |
| `cmd/` | CLI surface, daemon lifecycle, SFTP start/stop |
| `store/` | Encrypted profile store (AES-256-GCM, atomic writes) |
| `keyring/` | Master key: GNOME Keyring or file backend |
| `sshclient/` | SSH dial, PTY, auth, known_hosts, proxy chains |
| `sftpproxy/` | Local SSH/SFTP proxy server |
| `internal/` | Config, history, progress UI, i18n, privacy |
| `completions/` | Shell completion scripts |

### Daemon Protocol

The daemon communicates with clients over a Unix domain socket
(`~/.config/qssh/<profile>.sock`). The protocol is JSON lines
(newline-delimited). Stream frames (stdout, stderr, stdin) use
base64-encoded payloads within JSON.

Clients send requests (`exec`, `mount`, `unmount`, `stop`, `ping`,
`stdin`, `stdin_eof`) and read response frames (`stdout`, `stderr`,
`exit`, `mounted`, `error`, `stopped`, `ping`).

---

<a id="file-layout"></a>

## File Layout

```text
~/.config/qssh/
├── config.json         # User config (key-value store)
├── store.json          # Encrypted credential profiles (AES-256-GCM)
├── store.key           # Master key (file backend) or opt-in mirror (store.mirror_key)
├── known_hosts         # SSH host keys (TOFU)
├── hostkey.log         # Audit log of accepted host keys (TOFU)
├── history.jsonl       # Connection history (size-capped)
├── sftp.json           # SFTP proxy state
├── sftp_host_key       # SFTP proxy host key (RSA)
├── <profile>.sock      # Daemon control socket (Unix, 0600)
└── <profile>.pid       # Daemon PID file (0600)
```

---

## Security Model

### At Rest

- Profiles are encrypted with **AES-256-GCM**, random 12-byte nonces.
- The 32-byte master key is stored in GNOME Keyring (preferred) or a
  `0600` file at `~/.config/qssh/store.key`.
- The keyring will **never** silently mint a new master key when an
  encrypted `store.json` already exists — it returns a clear error instead.
- `store.json` and `store.key` are written atomically (temp + rename + fsync).

### In Transit

- Standard SSH transport with TOFU host key verification.
- On first use, the host key fingerprint is logged to `hostkey.log` and
  printed to stderr.

### Daemon Socket

- Unix domain socket with `0600` permissions.
- On Linux, `SO_PEERCRED` rejects connections from other UIDs.

### SFTP Proxy

- Binds `127.0.0.1` by default (loopback only).
- Accepts any password (local-only proxy; not a security boundary).

### Privacy

- Default-on host/IP redaction in UI output (list, progress, errors).
- Not a security boundary — actual network traffic still uses real addresses.

---

## Keyring Backend Behaviour

| Backend | First run | Keyring locked | Keyring unlocked |
|---------|-----------|----------------|------------------|
| `file` | Generates `store.key` | Uses `store.key` (no keyring needed) | Same |
| `keyring` | Stores key in keyring | Fails with clear error + recovery instructions | Reads from keyring |

By default the `keyring` backend keeps the master key **only** in the
keyring — a plaintext `store.key` is never created, so the keyring's
locked/unlocked state is the real protection boundary. Set
`store.mirror_key=true` to also mirror the key to `store.key` as a reboot
recovery aid; understand that the mirror is readable without unlocking the
keyring (any file-read access to the user's config directory decrypts the
store).

When `store.backend=keyring` and the keyring is locked after reboot,
qssh refuses to mint a new key and prints:

```text
encryption key not available (login keyring locked or missing entry) and no store.key found,
but encrypted store exists.
Unlock your session keyring, or restore ~/.config/qssh/store.key.
```

The fallback recovery path is: `qssh --config set store.backend file`
after restoring a valid `store.key`.
