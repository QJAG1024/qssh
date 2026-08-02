# AGENT.md — 开发协作规范

本项目由人类 + AI 协作开发。本文件约定开发流程与质量规范，供贡献者和
AI 助手遵守。

## 项目概况

- **QSSH**：SSH 凭据管理器（Go）。凭据 AES-256-GCM 加密存储，支持
  SSH 连接、`--exec` 远程命令、SFTP 代理、WebDAV 挂载、凭据导入导出。
- 语言：Go 1.26+。依赖极简（x/crypto、pkg/sftp、x/term）。
- 文档：`docs/{en-us,zh-cn}/`（双语）、`README.md`（中文概览）、
  `README.en.md`。
- 平台：Linux / macOS / Windows。`//go:build` 分平台文件注意保持同步。

## 开发流程

1. **改前读**：修改文件前先读完整上下文，不凭记忆改。
2. **小步提交**：每个逻辑单元一个 commit，提交信息说明"为什么"。
3. **自测**：`go build ./... && go vet ./... && go test -count=1 ./...`
   必须全绿；涉及平台差异时 `GOOS=windows go build ./...` 和
   `GOOS=darwin go build ./...` 交叉编译验证。
4. **CI 前自查**：push 前确认三平台编译 + 全量测试，避免 CI 挂。

## 质量规范

### i18n（必须）

- **所有用户可见字符串必须走 `i18n.T(key)`**（en-US + zh-CN 双份 key）。
- key 命名：`<domain>.<purpose>`（如 `webdav.failed`、`config.key.term_mode`）。
- 两个 locale 文件 key 必须成对（`internal/i18n/i18n_test.go` 的对账测试
  会强制校验，漏 key 直接红）。
- **HTTP 响应体 / 协议错误保持英文**（给客户端看的协议语义，不翻译）。
- 交互提示：`internal.Prompt` / `SelectPrompt` / `Confirm` / `ReadPassword`
  的 label 传 i18n key 翻译后的值。

### 测试（新功能必须带）

- 纯逻辑：单测（如 `shellQuote` 注入中和、`parseRange`、option 白名单）。
- 平台相关：测试用 `QSSH_*` env 隔离（`QSSH_CONFIG_PATH`、`QSSH_SFTP_STATE`
  等），**不要用 `XDG_CONFIG_HOME`**（macOS/Windows 不识别，CI 会挂）。
- 涉及真实 SSH：本地 fake-sshd（`/tmp/qssh-selftest/`）或真实 loopback。
- benchmark 放 `internal/bench_test.go` / `store/bench_test.go`（`-bench` 才跑）。

### 安全（敏感操作）

- 凭据（密码/密钥口令）**禁止走 argv flag**——会用 `--password` 之类时
  必须加 argv 警告（`cred.argv_warning`），优先交互提示或 stdin。
- 加密用 AES-256-GCM + 随机 nonce；口令派生用 PBKDF2（600k 迭代）。
- 绑定地址默认 loopback，非 loopback 必须有明确授权（token / allow 配置）。
- 进程终止：验证 PID 身份（starttime/exe）再 signal，防杀错进程。

### 架构约束

- **option 体系统一**：per-profile 键走 `--set-option` 白名单
  （`internal/option.go`），解析用 `EffectiveOption`（profile > 全局 > 默认）。
  新增可 per-profile 键要：白名单 + config 面板 + `--edit` 面板 + i18n + 文档。
- **补全从 main.go 自动生成**：新增 flag 后跑 `go run cmd/gen_completions.go`，
  不要手改 `completions/`（会被下次生成覆盖）。
- WebDAV / SFTP 代理 / daemon 的交互字符串 i18n 化；CLI 输出（URL 等）走 i18n。

### 提交信息风格

```
<type>: <简述>

- 细节 1
- 细节 2（含验证结果/数据）
```

`type`: feat / fix / refactor / perf / test / docs / i18n / style。

## 测试命令速查

```bash
go build ./...                    # 编译
go vet ./...                      # 静态检查
go test -count=1 ./...            # 全量测试
GOOS=windows go build ./...       # Windows 编译
GOOS=darwin go build ./...        # macOS 编译
go run cmd/gen_completions.go     # 重新生成 bash/zsh/fish 补全
go test ./internal/i18n/          # i18n key 对账
```

## 版本发布

- 版本号由 git tag 注入（CI 用 `git describe --tags`）。
- 发布：`git tag vX.Y.Z && git push origin vX.Y.Z`，CI 自动建 release。
