# 命令参考

<a id="profile-management"></a>

## 凭据管理

### `qssh --add`

创建新凭据。默认交互式；当提供 `--host` / `--user` / `--auth` 中任意一个时，进入非交互模式（AI agent 友好）。

```bash
# 交互式
qssh --add myserver

# 非交互式（agent 友好）
qssh --add myserver --host 192.168.1.1 --user root --auth password --password "xxx"
qssh --add myserver --host example.com --user deploy --auth key --key-path ~/.ssh/id_ed25519
qssh --add myserver --host example.com --user deploy --auth agent
```

| 选项 | 说明 |
| ------ | ------ |
| `--host <host>` | SSH 主机名或 IP 地址 |
| `--port <port>` | SSH 端口（默认: `22`） |
| `--user <user>` | SSH 用户名 |
| `--auth <type>` | 认证方式: `password`, `key`, `agent`, `keyboard-interactive` |
| `--password <pass>` | 密码（`--auth password` 时使用） |
| `--key-path <path>` | 私钥路径（`--auth key` 时使用） |
| `--key-passphrase <pass>` | 加密私钥的口令 |
| `--proxy <profile>` | 跳板机凭据名称 |
| `--set-option KEY=VALUE,...` | 逗号分隔的 SSH 选项 |
| `--tags tag1,tag2,...` | 逗号分隔的标签 |

### `qssh --edit`

编辑已有凭据。选项与 `--add` 相同。

```bash
qssh --edit myserver --host newhost.example.com
qssh --edit myserver --password newpass
qssh --edit myserver --proxy gateway
qssh --edit myserver --tags prod,web
```

### `qssh --delete`

删除凭据。默认需确认，用 `--yes` 跳过。

```bash
qssh --delete myserver
qssh --delete myserver --yes
```

### `qssh --copy` / `qssh --rename`

```bash
qssh --copy myserver myserver-backup
qssh --rename myserver new-server
```

### `qssh --list`

列出凭据。可选关键词过滤。`--json` 输出机器可读 JSON。

```bash
qssh --list
qssh --list prod          # 按名称或主机过滤
qssh --list --json        # 机器可读（密码已脱敏，隐私模式下隐藏主机）
```

### 凭据选项 (`--set-option`)

| 键 | 值 | 说明 |
| ---- | ----- | ------ |
| `ConnectTimeout` | 时长（如 `30s`） | TCP+SSH 握手超时。默认: `10s` |
| `SetEnv` | `KEY=VALUE,KEY2=VALUE2` | 远程会话环境变量 |

```bash
qssh --add srv --host 10.0.0.1 --user root --auth password --password x \
     --set-option ConnectTimeout=30s,SetEnv=LANG=en_US.UTF-8
```

---

## 认证方式

| 方式 | CLI 选项 | 所需 |
| ------ | ---------- | ------ |
| `password` | `--auth password` | `--password` |
| `key` | `--auth key` | `--key-path`；可选 `--key-passphrase` |
| `agent` | `--auth agent` | `SSH_AUTH_SOCK` |
| `keyboard-interactive` | `--auth keyboard-interactive` | 交互式提示 |

---

## 连接

```bash
qssh <profile>
```

打开带 PTY、信号转发和窗口大小调整的交互式 Shell。连接过程显示 DNS 解析、TCP 连接、SSH 握手、PTY 分配和 Shell 启动的进度。

---

<a id="remote-command-execution"></a>

## 远程命令执行

```bash
qssh --exec <profile> <command...>
```

### 行为

- **热路径**（daemon 运行中）：< 100 ms 完成。
- **冷启动**：自动 fork 托管 daemon，建立 SSH，执行命令。daemon 空闲 5 分钟后自动退出。
- **退出码**：传播到本地进程。
- **Stdin**：管道输入时转发（非 TTY）。二进制安全。
- **Args**：多个参数逐个 shell 引号；单个参数视为完整 shell 命令以保持向后兼容。

### 示例

```bash
# 简单命令
qssh --exec srv "uptime"

# 多参数（空格安全引号）
qssh --exec srv printf '%s\n' 'hello world'

# 管道 stdin
echo "data" | qssh --exec srv cat

# 二进制 stdin
tar czf - . | qssh --exec srv 'tar xzf - -C /tmp'
```

---

<a id="sftp-proxy"></a>

## SFTP 代理

本地 TCP 服务器，代理 SFTP 到远程主机。任何 SFTP 客户端均可连接。

```bash
# 启动（随机端口）
qssh --sftp-start srv
# → SFTP proxy: sftp://127.0.0.1:33803

# 指定绑定地址/端口
qssh --sftp-start srv --bind 127.0.0.1 --port 22222

# 停止
qssh --sftp-stop srv
```

如果 daemon 正在运行，SFTP 代理会复用其 SSH 连接。

**绑定授权。** 代理接受任意密码（真实认证在远端 SSH 连接），因此绑定地址是安全边界。三种非 loopback 绑定方式：

| 来源 | 行为 |
| ------ | ------ |
| `--bind 0.0.0.0`（CLI） | 警告 2 秒后放行——你显式要求了 |
| profile `sftp.bind=0.0.0.0` | 直接允许——per-profile 的选择本身就是授权 |
| 全局 `sftp.bind=0.0.0.0` | 拒绝启动，除非同时设置 `sftp.allow_non_loopback=true` |

设置非 loopback 的全局 `sftp.bind` 时配置阶段不拦截，但 `--sftp-start` 会拒绝启动直到 `sftp.allow_non_loopback=true`。

`--sftp-allow-remote` **已弃用**（保留以兼容脚本）：非 loopback 绑定现在由 `--bind` 或 per-profile `sftp.bind` 授权。

---

## 守护进程（连接复用）

| 模式 | 触发方式 | 生命周期 |
|------|----------|----------|
| **托管** | `--exec` 自动启动 | 空闲 5 分钟，自动退出 |
| **持久** | `qssh --daemon-start <profile>` | 直到 `--daemon-stop` |

```bash
qssh --daemon-start srv
qssh --daemon-stop srv
```

Daemon 每 30 秒发送 SSH keepalive 探测，连接断开时自动重连（SFTP 挂载期间除外）。

---

<a id="jump-hosts"></a>

## 跳板机（代理链）

凭据可以指定另一个凭据作为跳板机。支持多级链。

```bash
# 创建网关
qssh --add gateway --host 192.168.1.1 --user root --auth key --key-path ~/.ssh/id_ed25519

# 创建网关后的目标
qssh --add internal --host 10.0.0.5 --user root --auth password --password secret --proxy gateway

# 连接——自动通过网关隧道
qssh internal
qssh --exec internal hostname
```

交互式连接、`--exec` 和 SFTP 均支持跳板链。

---

## 历史记录

```bash
qssh --history               # 所有凭据
qssh --history myserver      # 单个凭据
qssh --last                  # 最近一条
```

历史记录受 `history.max_size` 限制（默认 5 MB）。详见 [config.md](config.md)。

### 命令记录

`--exec` 命令有多少写入历史，由三级模式控制。安全的默认值（`masked`）只保存命令名（`docker compose up -d` → `docker`），因此作为参数传入的密钥永远不会落到磁盘。

| 模式 | 行为 |
| ------ | ------ |
| `full` | 保存完整命令行 |
| `masked`（默认） | 只保存第一个 token（命令名） |
| `off` | 完全不保存命令 |

```bash
# 全局默认
qssh --config set history.record_commands full

# 按凭据覆盖（优先于全局）
qssh --add myserver --host 1.2.3.4 --set-option history.record_commands=off
qssh --edit myserver --set-option history.record_commands=masked
```

---

<a id="privacy"></a>

## 隐私

主机/IP 地址**默认**从 UI 输出中脱敏。

```bash
qssh --privacy               # 显示状态
qssh --privacy off            # 显示主机（重启前粘性）
qssh --privacy on             # 隐藏主机（粘性）
qssh --privacy clear          # 重置为默认 on
qssh --reveal --list           # 仅本次进程显示主机
```

启用时，`--list` 隐藏 Host 列，JSON 省略 `host`，进度/错误消息中显示 `***`。

---

## 配置

```bash
qssh --config                  # 列出所有键
qssh --config get <key>        # 获取值
qssh --config set <key> <val>  # 设置值
qssh --config unset <key>      # 移除（恢复默认）
```

完整配置键参考: [config.md](config.md)。

---

## 版本

```bash
qssh --version
```
