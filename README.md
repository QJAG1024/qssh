<div align="center">

<h1>QSSH</h1>
<h3>终端中的简单快速的SSH凭据管理器</h3>

中文 | [English](./README.en.md)

</div>

```
qjag186@QJAG-Legion-EOS ~> ./qssh homelab
Profile: homelab (root@192.168.10.139:22)
  ✔ Profile loaded
  → Resolving 192.168.10.139
  ✔ DNS 解析 (192.168.10.139 → 192.168.10.139 (0ms))
  → Connecting to 192.168.10.139:22
  ✔ SSH 握手 (Connected in 26ms)
  → PTY 分配
  ✔ PTY 分配
  → 启动 Shell
  ✔ Session established, entering interactive mode
```

无须打开专门的桌面SSH客户端，也无须每次输入密码，只需要一行命令即可使用你熟悉的终端模拟器配置连接到你的主机。

## 安装

```bash
go build -o qssh .
```

## 用法

### 添加凭据

```bash
# 交互式添加
./qssh --add myserver

# 单行添加（AI agent 友好）
./qssh --add myserver --host 192.168.1.1 --user root --auth password --password "xxx"
./qssh --add myserver --host 192.168.1.1 --user root --auth key --key-path ~/.ssh/id_ed25519
./qssh --add myserver --host example.com --user deploy --auth agent
```

交互式填写 Host、Port、User、认证方式。

支持四种认证方式：

| 方式 | 说明 |
|---|---|
| `password` | 密码认证，密文存储 |
| `key`   | 私钥路径（可选加密口令） |
| `agent` | SSH Agent (SSH_AUTH_SOCK) |
| `keyboard-interactive` | 交互式认证（如 2FA） |

### 连接

```bash
./qssh myserver
```

连接过程显示逐步状态：DNS 解析、TCP 连接、SSH 握手、认证、PTY 分配、启动 Shell。

### 远程命令执行

在远程主机上执行一条命令并返回退出码。

```bash
./qssh --exec myserver "uptime"
./qssh --exec myserver "uname -a"
./qssh --exec myserver "systemctl status sshd"
```

首次执行时自动启动托管守护进程（managed daemon），保持 SSH 连接复用。后续调用瞬间完成，无需重复认证。守护进程空闲 5 分钟后自动退出。

特别适合 AI agent、脚本、自动化场景——只需调用 `--exec`，其余由工具管理。

### 远程文件访问

两种方式，按客户端选择：

**SFTP 代理**（FileZilla / cyberduck 等专业 SFTP 客户端）

```bash
./qssh --sftp-start myserver
# → SFTP proxy: sftp://127.0.0.1:33125
./qssh --sftp-stop myserver
```

**WebDAV 挂载**（系统文件管理器原生挂载：macOS Finder、Windows 资源管理器、Linux gvfs/KDE）

```bash
./qssh --webdav-start myserver
# → WebDAV:
#     dav://127.0.0.1:34123/
#     http://127.0.0.1:34123/
./qssh --webdav-stop myserver
```

挂载方式（按系统）：
- **macOS**：Finder → 前往 → 连接服务器 → 输入 `http://127.0.0.1:端口/`
- **Windows**：资源管理器 → 映射网络驱动器 → 输入 `http://127.0.0.1:端口/`
- **Linux**：文件管理器地址栏输入 `dav://127.0.0.1:端口/`（或 `gio mount`）

非 loopback 绑定（`--bind 0.0.0.0` 等）时自动启用 token 认证，输出带凭据的 URL。

### 凭据导入导出

把 profile（含密码/密钥）导出为口令加密的 `.qssh` 文件，跨机器迁移。

```bash
./qssh --export myserver              # 交互：询问口令 + 目录
printf '口令\n' | ./qssh --export myserver --dir ~/backups
./qssh --import myserver.qssh --name myserver
```

### 守护进程（后台连接复用）

| 模式 | 说明 |
|---|---|
| `managed`（托管） | `--exec` 自动启动，空闲 5 分钟自动退出 |
| `persistent`（持久） | 手动 `--daemon-start` / `--daemon-stop`，长期驻留 |

### 更多功能

- **跳板机**：配置 `--proxy` 自动走多级跳板
- **隐私模式**：默认对 UI 输出中主机/IP 脱敏，可通过 `--reveal` 临时查看
- **Agent 友好**：`--yes` 跳过确认、`--list --json` 机器可读输出、`--exec` 支持 stdin 管道
- **主机密钥**：TOFU（首次使用时接受）并记录指纹审计日志
- **Per-profile 选项**：`--set-option` 按 profile 覆盖全局配置（term.mode / hostkey.mode / sftp.bind 等）
- **命令历史记录**：`--exec` 命令默认只记命令名（`history.record_commands` 可调 full/masked/off）
- **补全**：`make completions` 生成 bash/zsh/fish 补全（从 main.go flag 自动同步）

## 完整文档

所有命令、配置键、架构说明请见[文档](docs/zh-cn/README.md)。

## 数据存储

- 凭据: `~/.config/qssh/store.json`（AES-256-GCM 加密）
- 主密钥: `~/.config/qssh/store.key` 或 GNOME Keyring（`secret-tool`）
- 已知主机: `~/.config/qssh/known_hosts`
- 守护进程: `~/.config/qssh/<profile>.sock`（Unix socket）
- 配置: `~/.config/qssh/config.json`

## 依赖

- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) — SSH 协议 + 主机密钥验证
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — 终端 raw mode
- [github.com/pkg/sftp](https://github.com/pkg/sftp) — SFTP 客户端及代理