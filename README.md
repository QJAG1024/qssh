<h1 align="center">QSSH</h1>
<h3 align="center">终端中的简单快速的SSH凭据管理器</h3>

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

首次执行时自动启动托管守护进程（managed daemon），保持 SSH 连接复用。后续调用瞬间完成，无需重复认证。守护进程空闲 5 分钟后自动退出，无需手动清理。

特别适合 AI agent、脚本、自动化场景——只需调用 `--exec`，其余由工具管理。

### 远程文件访问（SFTP 代理）

启动本地 SFTP 透明代理，可作为远程 SFTP 的中转。任何 SFTP 客户端均可连接使用（FileZilla、Cyberduck、`sftp` 命令行等）。

```bash
# 启动 SFTP 代理（监听随机端口）
./qssh --sftp-start myserver

# 指定绑定地址
./qssh --sftp-start myserver --bind 127.0.0.1

# 指定端口
./qssh --sftp-start myserver --bind 127.0.0.1 --port 22222

# 停止 SFTP 代理
./qssh --sftp-stop myserver
```

代理接受任意密码作认证（透明转发），SSH 连接复用已有通道。

如果后台守护进程正在运行，SFTP 代理会自动复用守护进程的连接。

### 守护进程（后台连接复用）

守护进程保持 SSH 连接不断开，其他操作可以复用该连接，省去重复认证的开销。

**两种模式：**

| 模式 | 说明 |
|---|---|
| `managed`（托管） | `--exec` 自动启动，空闲 5 分钟自动退出，无需手动管理 |
| `persistent`（持久） | 手动 `--daemon-start` / `--daemon-stop`，长期驻留 |

```bash
# 启动持久守护进程
./qssh --daemon-start myserver

# 复用守护进程执行命令或启动 SFTP
./qssh --exec myserver "uptime"
./qssh --sftp-start myserver

# 停止持久守护进程
./qssh --daemon-stop myserver
```

托管模式无需显式启动/停止守护进程——`--exec` 会自动处理。多次连续调用时复用同一连接，效率更高。

### 管理

```bash
./qssh --list [filter]              # 列出凭据，可选关键词过滤
./qssh --edit myserver              # 修改凭据
./qssh --delete myserver            # 删除凭据
./qssh --config [get|set ...]       # 查看或修改设置
./qssh --sftp-start myserver        # 启动 SFTP 代理
./qssh --sftp-stop myserver         # 停止 SFTP 代理
./qssh --exec myserver <cmd>        # 远程执行命令
./qssh --daemon-start myserver      # 启动后台守护进程
./qssh --daemon-stop myserver       # 停止后台守护进程
./qssh --version                    # 查看版本
```

## 数据存储

- 凭据文件: `~/.config/qssh/store.json`（AES-256-GCM 加密）
- 主密钥: 优先用 `secret-tool`（GNOME Keyring），回退到 `~/.config/qssh/key`
- 已知主机: `~/.config/qssh/known_hosts`
- 守护进程: `~/.config/qssh/<profile>.sock`（Unix socket）
- SFTP 状态: `~/.config/qssh/sftp.json`

## 依赖

- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) — SSH 协议 + 主机密钥验证
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — 终端 raw mode
- [github.com/pkg/sftp](https://github.com/pkg/sftp) — SFTP 客户端及代理