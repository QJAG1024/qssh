# WebDAV Performance Design Decision

## Background

An early attempt bridged SFTP through the stock `x/net/webdav` library
(`webdav.FileSystem`) to implement mounting. On **high-latency links**
(200ms+ RTT remote hosts) this had severe performance problems:

- `walkFS` issues an extra `fs.Stat` per directory entry while listing,
  ignoring the attributes `Readdir` already returned — one PROPFIND
  becomes **N remote round-trips**.
- Property finders (e.g. `getcontenttype`) `OpenFile` + read the file
  header per entry to guess the MIME type — the same per-entry round-trip.
- Measured: PROPFIND on a 210-item directory took **~50s**
  (~200ms round-trip × 210+).

## Decision: hand-written PROPFIND, one ReadDir per response

QSSH's WebDAV implementation does **not depend on x/net/webdav**; it is a
minimal RFC 4918 handler of its own:

```
PROPFIND dir → one sftp.ReadDir() → build MultiStatus response from the
returned os.FileInfo (mode/size/modtime/IsDir)
```

- **Zero per-entry round-trips**: one ReadDir returns every entry's
  attributes; no second Stat/Open pass.
- **getcontenttype guessed from the file extension**
  (`mime.TypeByExtension`), never reads the file.
- Measured: PROPFIND on a 210-item directory **~1.1s** (≈ one SFTP
  round-trip — at the protocol floor).

## Benchmark (213ms RTT host, /etc with 210 entries)

| Approach | PROPFIND / ls -l latency |
| --- | --- |
| Stock x/net/webdav (per-entry stat) | ~50s |
| Raw SFTP per-entry stat (baseline) | ~33s |
| sshfs (FUSE) | ~1.7s |
| **QSSH hand-written WebDAV (one ReadDir)** | **~1.1s** |

## Additional optimizations

- **Concurrent writes** (`ReadFromWithConcurrency`): uploads 52s → 6.7s
  (10MB @ 213ms RTT), avoiding `io.Copy` degrading to per-packet ACK waits.
- Full scheme comparison: `archive/MOUNT_EXPERIMENTS_REPORT.md`.
