package i18n

var zhCN = map[string]string{
	// Meta
	"locale.code": "zh-CN",

	// Step labels
	"step.decrypt":       "凭据解密",
	"step.dns_resolve":   "DNS 解析",
	"step.tcp_connect":   "TCP 连接建立",
	"step.ssh_handshake": "SSH 握手",
	"step.authenticate":  "认证",
	"step.allocate_pty":  "PTY 分配",
	"step.shell_start":   "启动 Shell",
	"step.proxy_connect": "代理连接",
	"step.unknown":       "未知步骤",

	// Session progress messages
	"profile.loaded":      "配置已加载",
	"resolving":           "正在解析 %s",
	"dns_resolve.failed":  "DNS 解析失败: %v",
	"dns_resolve.hint":    "请检查配置文件中的主机名或 IP 地址",
	"dns_resolve.detail":  "%s → %s (%dms)",
	"connecting":          "正在连接 %s",
	"tcp_connect.failed":  "TCP 连接失败: %s",
	"tcp_connect.hint":    "请确认主机在线、端口正确、防火墙已放行",
	"authenticate.failed": "认证失败: %v",
	"authenticate.hint":   "请检查配置中的凭据: qssh --edit %s",
	"connected":           "已连接 (%dms)",
	"pty_allocate.failed": "PTY 分配失败: %v",
	"shell_start.failed":  "Shell 启动失败: %v",
	"session.ready":       "会话已建立，进入交互模式",

	// Profile CRUD
	"store.open_error":          "打开存储失败: %v",
	"profile.not_found":         "配置 %q 不存在",
	"profile.exists":            "配置 %q 已存在。请使用 'qssh --edit' 修改",
	"profile.created":           "配置 %q 已创建。使用 'qssh %s' 连接",
	"profile.updated":           "配置 %q 已更新",
	"profile.deleted":           "配置 %q 已删除",
	"profile.delete_confirm":    "删除配置 %q？",
	"profile.cancelled":         "已取消",
	"profile.copied":            "配置 %q 已复制到 %q",
	"profile.renamed":           "配置 %q 已重命名为 %q",
	"profile.list_empty":        "没有配置。使用 'qssh --add <name>' 创建",
	"profile.list_empty_filter": "没有匹配 %q 的配置",
	"copy.error":         "复制配置失败: %v",
	"rename.error":       "重命名配置失败: %v",
	"rename.store_error": "打开配置库失败: %v",
	"history.error":      "读取历史失败: %v",
	"field.required_host":       "主机为必填项",
	"field.required_user":       "用户为必填项",
	"field.edit_header":         "正在编辑配置 %q（回车保持原值）",
	"auth.unsupported":          "不支持的认证方式 %q",
	"add.required_password":     "password auth 需要提供 --password",
	"add.required_keypath":      "key auth 需要提供 --key-path",
	"password.read_error":       "读取密码失败: %v",
	"password.confirm_suffix": "（确认）",
	"password.mismatch":       "两次密码输入不一致",
	"prompt.select":           "选择",
	"prompt.confirm_no":       " [y/N]: ",
	"prompt.confirm_yes":      " [Y/n]: ",
	"password.change_prompt":    "更改密码？",
	"password.new_prompt":       "新密码",
	"profile.save_error":        "保存配置失败: %v",

	// Proxy / Jump host
	"proxy.connecting": "正在连接跳板机 %s...",
	"proxy.tunneling":  "通过 %s 隧道至 %s",
	"proxy.handshake":  "通过 %s 进行目标 SSH 握手",

	// Connection history
	"history.header":     "%s 的连接历史",
	"history.header_all": "连接历史",
	"history.time":       "时间",
	"history.duration":   "耗时",
	"history.command":    "命令",
	"history.exit":       "退出码",
	"history.empty":      "没有找到记录。",
	"history.empty_all":  "没有找到连接记录。",

	// Connection
	"connect.failed":         "连接失败",
	"connect.ended":          "会话结束: %v",
	"profile.header":         "配置: %s (%s@%s:%d)",
	"profile.header_private": "配置: %s (%s)",
	"session.closed":         "  ⚡ 连接已关闭 (%s)",
	"privacy.status":         "隐私模式: %s（来源: %s）",
	"privacy.set":            "隐私模式粘性设置为 %s（重启前有效）",
	"privacy.cleared":        "隐私模式粘性已清除（默认: 开启）",
	"privacy.usage":          "用法: qssh --privacy [on|off|clear|status]",

	// Config
	"config.usage.get":      "用法: qssh --config get <key>",
	"config.usage.set":      "用法: qssh --config set <key> <value>",
	"config.usage.unset":    "用法: qssh --config unset <key>",
	"config.unknown_action": "未知的 config 操作 %q（使用 get/set/unset）",
	"config.empty":          "（无设置）",
	"config.not_set":        "（未设置）",
	"config.set":            "%s = %s",
	"config.unset":          "%s 已删除",
	"config.save_error":     "保存设置失败: %v",

	// SFTP
	"sftp.preparing":      "正在准备...",
	"sftp.opening_store":  "正在打开存储...",
	"sftp.connecting":     "正在连接 SSH...",
	"sftp.starting":       "正在启动 SFTP...",
	"sftp.starting_proxy": "正在启动 SFTP 代理...",
	"sftp.proxy_started":  "SFTP 代理: %s",
	"sftp.failed":         "SFTP 启动失败: %v",
	"sftp.stop_failed":    "SFTP 停止失败: %v",
	"sftp.stopped":        "SFTP 已停止",
	"sftp.daemon_failed":  "守护进程启动失败",
	"sftp.mount_active": "SFTP 代理正在运行（挂载激活），请先卸载或强制停止",

	// SFTP 绑定授权
	"sftp.bind.warn_cli":         "警告：SFTP 代理绑定到 %s（非 loopback）。代理接受任意密码——远程服务器的文件系统将从网络可达。2 秒后继续...",
	"sftp.bind.deprecated_flag":  "警告：--sftp-allow-remote 已弃用且不再需要；非 loopback 绑定由 --bind 或 per-profile sftp.bind 授权。",
	"sftp.bind.refuse_global":    "拒绝启动：全局 sftp.bind=%s 为非 loopback，但 sftp.allow_non_loopback 未设为 true。",
	"sftp.bind.refuse_hint":      "如果该 profile 需要监听非 loopback 地址，请在 profile 上设置 sftp.bind（per-profile 的选择本身就是授权）。否则设置 sftp.allow_non_loopback=true 以全局接受风险。",
	"sftp.bind.set_warn":         "警告：全局 sftp.bind 为非 loopback。除非 sftp.allow_non_loopback=true，否则 qssh 将拒绝启动此类绑定。",
	"sftp.bind.set_hint_allow":   "如果你了解风险，请运行: qssh --config set sftp.allow_non_loopback true",
	"sftp.bind.set_hint_profile": "提示：改为在单个 profile 上设置 sftp.bind（per-profile 的选择本身就是授权）：qssh --edit <profile> --set-option sftp.bind=%s",

	// 配置交互面板
	"config.panel.title":         "QSSH 配置",
	"config.panel.not_set":       "（未设置）",
	"config.panel.set":           "s) 设置一个键",
	"config.panel.unset":         "u) 删除一个键",
	"config.panel.quit":          "q) 退出",
	"config.panel.action":        "操作",
	"config.panel.key":           "键",
	"config.panel.value":         "值",
	"config.panel.unset_which":   "删除哪个键？",
	"config.panel.unset_confirm": "删除此键？",
	"config.panel.remove_all":    "删除所有选项？",
	"config.no_keys":             "没有可删除的键",

	// 交互式添加
	"add.panel.title":        "正在创建凭据: %s",
	"add.prompt.host":        "主机",
	"add.prompt.port":        "端口",
	"add.prompt.user":        "用户",
	"add.prompt.auth":        "认证方式:",
	"add.prompt.keypath":     "密钥路径",
	"add.prompt.keypass":     "密钥口令",
	"add.prompt.password":    "密码",
	"add.prompt.proxy":       "跳板凭据（可选）",
	"add.prompt.options":     "选项（逗号分隔 KEY=VALUE，可选）",
	"add.prompt.tags":        "标签（逗号分隔，可选）",
	"add.prompt.save":        "保存凭据？",
	"add.confirm.keypass":    "密钥有口令？",
	"add.warn.key_missing":   "警告：密钥文件 %q 不存在: %v",
	"add.warn.proxy_missing": "警告：跳板凭据 %q 不存在，稍后可创建",
	"add.error.self_proxy":   "错误：跳板不能指向自身",
	"add.preview.title":      "预览",
	"add.preview.name":       "名称",
	"add.preview.host":       "主机",
	"add.preview.user":       "用户",
	"add.preview.auth":       "认证",
	"add.preview.password":   "密码",
	"add.preview.set":        "（已设置）",
	"add.preview.keypath":    "密钥路径",
	"add.preview.passphrase": "口令",
	"add.preview.agent":      "（SSH agent）",
	"add.preview.proxy":      "跳板",
	"add.preview.options":    "选项",
	"add.preview.tags":       "标签",

	// 交互式编辑
	"edit.panel.title":         "正在编辑: %s  (%s@%s:%d)",
	"edit.menu.host":           "主机/端口/用户",
	"edit.menu.auth":           "认证方式与凭据",
	"edit.menu.proxy":          "跳板",
	"edit.menu.options":        "选项",
	"edit.menu.tags":           "标签",
	"edit.menu.save":           "保存并退出",
	"edit.menu.discard":        "放弃",
	"edit.prompt.choose":       "选择",
	"edit.error.invalid":       "无效选择",
	"edit.prompt.host":         "主机",
	"edit.prompt.port":         "端口",
	"edit.prompt.user":         "用户",
	"edit.prompt.proxy":        "跳板凭据（留空删除）",
	"edit.prompt.options":      "选项（逗号分隔 KEY=VALUE）",
	"edit.prompt.tags":         "标签（逗号分隔）",
	"edit.prompt.auth":         "认证方式:",
	"edit.prompt.keypath":      "密钥路径",
	"edit.prompt.keypass":      "密钥口令",
	"edit.prompt.newpass":      "新密码",
	"edit.confirm.changepass":  "修改密码？",
	"edit.confirm.keypass":     "密钥有口令？",
	"edit.confirm.remove_proxy": "删除跳板？",
	"edit.confirm.remove_opts":  "删除所有选项？",
	"edit.confirm.save":         "保存更改？",
	"edit.confirm.discard":      "放弃所有更改？",
	"edit.error.options":        "选项: %v",

	// 守护进程
	"daemon.already_running": "daemon 已在运行",
	"daemon.started":         "daemon 已为 %q 启动",
	"daemon.stopped":         "daemon 已为 %q 停止",

	// List table headers
	"list.header.name":      "名称",
	"list.header.host":      "主机",
	"list.header.port":      "端口",
	"list.header.user":      "用户",
	"list.header.auth":      "认证",
	"list.header.last_used": "上次使用",
	"list.header.count":     "次数",
	"list.header.proxy":     "跳板",

	// Time
	"time.just_now":    "刚刚",
	"time.minutes_ago": "%d 分钟前",
	"time.hours_ago":   "%d 小时前",

	// Usage
	"usage.text": `QSSH - SSH 凭据管理器 v%s

用法:
  qssh <profile>                    连接配置
  qssh --add <name>                 创建新配置
  qssh --edit <name>                编辑已有配置
  qssh --list [filter]              列出配置（可选子串过滤）
  qssh --copy <old> <new>           复制配置
  qssh --rename <old> <new>         重命名配置
  qssh --delete <name>              删除配置
  qssh --exec <profile> <command>   在配置上执行命令
  qssh --sftp-start <name>          启动 SFTP 代理
  qssh --sftp-stop <name>           停止 SFTP 代理
  qssh --daemon-start <name>        启动后台守护进程
  qssh --daemon-stop <name>         停止后台守护进程
  qssh --history [name]             查看连接历史
  qssh --config [get|set ...]       查看或修改设置
  qssh --version                    显示版本`,
}
