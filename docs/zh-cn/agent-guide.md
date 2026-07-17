# AI Agent 指南

QSSH 为 agent 驱动的工作流设计。所有操作均可非交互执行。

## 核心模式

### 列出并过滤

```bash
# 机器可读 JSON（密码已脱敏，隐私模式下隐藏主机）
qssh --list --json

# 按凭据名称或主机过滤
qssh --list --json | jq '.[] | select(.name | test("prod"))'
```

### 执行命令

```bash
# 简单命令
qssh --exec srv "systemctl status nginx"

# 多参数带空格
qssh --exec srv printf '%s\n' 'hello world'

# 管道传输数据
echo "SELECT 1" | qssh --exec db "mysql -e '$(cat)'"

# 二进制数据
tar czf - /data | qssh --exec backup "cat > /backup/data.tgz"
```

### 创建凭据

```bash
qssh --add srv --host 10.0.0.1 --port 22 --user root --auth password --password "xxx"
qssh --add srv --host 10.0.0.1 --user root --auth key --key-path ~/.ssh/id_ed25519
qssh --add srv --host 10.0.0.1 --user root --auth key --key-path ~/.ssh/id_ed25519 --key-passphrase "secret"
qssh --add behind-gw --host 10.0.0.5 --user root --auth password --password x --proxy gateway
qssh --add srv --host 10.0.0.1 --user root --auth password --password x --tags prod,web
```

### 无确认删除

```bash
qssh --delete old-server --yes
qssh --delete old-server -y
```

### 调试时显示主机

```bash
qssh --reveal --list --json   # 一次性：本次进程显示 IP
qssh --privacy off             # 粘性：重启前一直显示 IP
```

### 检查远程系统状态

```bash
qssh --exec srv "uptime && free -m && df -h /"
qssh --exec srv "systemctl is-active nginx docker"
```

### 部署软件

```bash
qssh --exec srv "cd /app && git pull && systemctl restart app"
```

### SFTP 代理传输文件

```bash
qssh --sftp-start srv
# → SFTP proxy: sftp://127.0.0.1:33803
# 然后使用任何 SFTP 客户端传输文件

qssh --sftp-stop srv
```

---

## Agent 设计属性

- **无交互提示**: `--exec` 自动启动托管 daemon。`--delete --yes` 跳过确认。`--add` 带选项非交互运行。
- **退出码**: `--exec` 传播远程退出码。agent 可根据成功/失败分支。
- **Stdin 转发**: 管道数据直接传输到远程命令。
- **二进制安全**: daemon 协议中使用 base64 编码帧。
- **连接复用**: 首次 `--exec` 后，daemon 保持 SSH 连接。后续调用 < 100 ms。
- **空闲清理**: 托管 daemon 空闲 5 分钟后自动退出。
- **隐私**: 列表输出和错误消息中默认隐藏主机。使用 `--reveal` 调试。
- **JSON 输出**: `--list --json` 生成可解析输出，密码已脱敏。

---

## 技巧

### 使用凭据名称作为稳定标识符

凭据名称是主要标识。Agent 应按名称引用凭据，而非 IP——主机在隐私模式下隐藏且可能变化。

### 优先使用 `--exec` 而非交互式 Shell

`qssh --exec srv "cmd"` 是 agent 的正确工具。`qssh srv` 打开交互式 PTY Shell——仅在有操作员时使用。

### 通过管道串联命令

```bash
# 运行序列
qssh --exec srv "cd /app && git pull && go build && systemctl restart app"

# 或通过 stdin 运行复杂脚本
cat deploy.sh | qssh --exec srv bash
```

### 长时间会话的 daemon 生命周期管理

如果需要在多个命令间保持持久连接：

```bash
qssh --daemon-start srv      # 启动持久 daemon
# ... 多次 --exec 调用 ...
qssh --daemon-stop srv       # 完成后停止
```

否则，让托管 daemon 自动处理即可。