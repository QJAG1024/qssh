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
Welcome to Ubuntu 24.04 LTS (GNU/Linux 6.17.2-1-pve x86_64)

 * Documentation:  https://help.ubuntu.com
 * Management:     https://landscape.canonical.com
 * Support:        https://ubuntu.com/pro
Last login: Sun May 10 06:52:25 2026 from 192.168.10.186
root@homelab:~# 
```

无须打开专门的桌面SSH客户端，也无须每次输入密码，只需要一行命令即可使用你熟悉的终端模拟器配置连接到你的主机。

## 安装

```bash
go build -o qssh .
```

## 用法

### 添加凭据

```bash
./qssh --add myserver
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

### 管理

```bash
./qssh --list [filter]    # 列出凭据，可选关键词过滤
./qssh --edit myserver    # 修改凭据
./qssh --delete myserver  # 删除凭据
./qssh --version          # 查看版本
```

## 数据存储

- 凭据文件: `~/.config/qssh/store.json`（AES-256-GCM 加密）
- 主密钥: 优先用 `secret-tool`（GNOME Keyring），回退到 `~/.config/qssh/key`
- 已知主机: `~/.config/qssh/known_hosts`

## 依赖

- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) — SSH 协议
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — 终端 raw mode