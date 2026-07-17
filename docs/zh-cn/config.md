# 配置

所有配置存储在 `~/.config/qssh/config.json` 中，键值对格式。使用 `qssh --config` 管理。

## 配置键

| 键 | 值 | 默认 | 说明 |
|----|--------|---------|------|
| `store.backend` | `file`, `keyring` | 自动探测 | 主密钥存储后端。`file` 使用 `~/.config/qssh/store.key`；`keyring` 使用 GNOME 密钥环（`secret-tool`）。已有 `store.key` 时优先 `file`。 |
| `lang` | `en-US`, `zh-CN` | `en-US` | 界面语言。 |
| `sftp.bind` | IP 地址 | `127.0.0.1` | `--sftp-start` 的默认绑定地址。 |
| `hostkey.mode` | `tofu`, `strict` | `tofu` | `tofu`: 首次使用时接受未知主机密钥（指纹记录到 `hostkey.log`）。`strict`: 拒绝未知主机。 |
| `term.mode` | `passthrough`, `compat` | `passthrough` | `passthrough`: 将本地 `$TERM` 原样发送到远程 PTY。`compat`: 强制 `xterm`（适用于缺少 `ncurses-term` 的主机）。 |
| `history.max_size` | 大小字符串 | `5M` | 历史文件最大大小。支持 `K`/`M`/`G` 后缀。超出时每次追加自动裁剪最旧条目。 |

### 大小字符串格式

`history.max_size` 接受：

| 格式 | 示例 | 含义 |
|------|------|------|
| 纯数字 | `1048576` | 1,048,576 字节 |
| K 后缀 | `500K` | 500 × 1024 = 512,000 字节 |
| M 后缀 | `5M` | 5 × 1024² = 5,242,880 字节 |
| G 后缀 | `1G` | 1 × 1024³ 字节 |

## 示例

```bash
qssh --config set lang zh-CN
qssh --config set hostkey.mode strict
qssh --config set history.max_size 10M
qssh --config set term.mode compat
qssh --config unset term.mode          # 恢复 passthrough
qssh --config get lang                 # 输出 "zh-CN"
```

---

<a id="environment-variables"></a>
## 环境变量

| 变量 | 默认值 | 说明 |
|------|---------|------|
| `QSSH_STORE_PATH` | `~/.config/qssh/store.json` | 覆盖凭据文件路径 |
| `QSSH_KEY_PATH` | `~/.config/qssh/store.key` | 覆盖密钥文件路径 |
| `QSSH_HISTORY_PATH` | `~/.config/qssh/history.jsonl` | 覆盖历史文件路径 |
| `QSSH_PRIVACY` | _(未设置)_ | 强制隐私模式: `on`, `off`（覆盖粘性，非配置） |
| `QSSH_KNOWN_HOSTS` | `~/.config/qssh/known_hosts` | 覆盖 known_hosts 文件路径 |
| `XDG_RUNTIME_DIR` | 系统默认 | 粘性隐私状态的运行时目录 |
| `SSH_AUTH_SOCK` | _(agent)_ | SSH agent 套接字（`--auth agent` 时使用） |

---

## 粘性隐私状态

`--privacy on|off` 写入 `$XDG_RUNTIME_DIR/qssh/privacy`（权限 `0600`）。此文件在重启时自动清除。它**不是**配置键——是独立于 `config.json` 的运行时状态。