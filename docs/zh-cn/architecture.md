# 架构

## 数据流

```text
┌──────────┐     ┌──────────┐     ┌──────────┐
│  store.json │───▶│  load()   │───▶│  profiles │
│  (加密)     │    │  decrypt  │    │  (内存)   │
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

### 核心包

| 包 | 职责 |
|----|------|
| `cmd/` | CLI 入口、daemon 生命周期、SFTP 启停 |
| `store/` | 加密凭据存储（AES-256-GCM、原子写入） |
| `keyring/` | 主密钥：GNOME 密钥环或文件后端 |
| `sshclient/` | SSH 拨号、PTY、认证、known_hosts、代理链 |
| `sftpproxy/` | 本地 SSH/SFTP 代理服务器 |
| `internal/` | 配置、历史、进度 UI、国际化、隐私 |
| `completions/` | Shell 补全脚本 |

### Daemon 协议

Daemon 通过 Unix 域套接字（`~/.config/qssh/<profile>.sock`）与客户端通信。协议为 JSON 行（换行分隔）。流帧（stdout、stderr、stdin）使用 base64 编码。

客户端发送请求（`exec`、`mount`、`unmount`、`stop`、`ping`、`stdin`、`stdin_eof`）并读取响应帧（`stdout`、`stderr`、`exit`、`mounted`、`error`、`stopped`、`ping`）。

---

<a id="file-layout"></a>
## 文件布局

```text
~/.config/qssh/
├── config.json         # 用户配置（键值存储）
├── store.json          # 加密凭据配置（AES-256-GCM）
├── store.key           # 主密钥（文件后端）或回退镜像
├── known_hosts         # SSH 主机密钥（TOFU）
├── hostkey.log         # 已接受主机密钥审计日志（TOFU）
├── history.jsonl       # 连接历史（大小受控）
├── sftp.json           # SFTP 代理状态
├── sftp_host_key       # SFTP 代理主机密钥（RSA）
├── <profile>.sock      # Daemon 控制套接字（Unix，0600）
└── <profile>.pid       # Daemon PID 文件（0600）
```

---

## 安全模型

### 静态存储
- 凭据使用 **AES-256-GCM** 加密，随机 12 字节 nonce。
- 32 字节主密钥存储在 GNOME 密钥环（首选）或 `~/.config/qssh/store.key`（`0600` 权限）。
- 密钥环**绝不会**在已有加密 `store.json` 时静默生成新密钥——会返回明确错误。
- `store.json` 和 `store.key` 原子写入（temp + rename + fsync）。

### 传输中
- 标准 SSH 传输，TOFU 主机密钥验证。
- 首次使用时，主机密钥指纹记录到 `hostkey.log` 并打印到 stderr。

### Daemon 套接字
- Unix 域套接字，`0600` 权限。
- Linux 上通过 `SO_PEERCRED` 拒绝其他 UID 的连接。

### SFTP 代理
- 默认绑定 `127.0.0.1`（仅本地回环）。
- 接受任意密码（本地代理，非安全边界）。

### 隐私
- 默认启用 UI 输出中的主机/IP 脱敏（列表、进度、错误）。
- 非安全边界——实际网络流量仍使用真实地址。

---

## 密钥环后端行为

| 后端 | 首次运行 | 密钥环锁定 | 密钥环解锁 |
|------|----------|------------|------------|
| `file` | 生成 `store.key` | 使用 `store.key`（无需密钥环） | 同上 |
| `keyring` | 在密钥环中存储密钥 | 报错并给出恢复指引 | 从密钥环读取，镜像到 `store.key` |

当 `store.backend=keyring` 且重启后密钥环锁定时，qssh 拒绝生成新密钥并提示：

```text
encryption key not available (login keyring locked or missing entry) and no store.key found,
but encrypted store exists.
Unlock your session keyring, or restore ~/.config/qssh/store.key.
```

回退恢复路径：恢复有效的 `store.key` 后执行 `qssh --config set store.backend file`。