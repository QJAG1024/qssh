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
	"profile.list_empty":        "没有配置。使用 'qssh --add <name>' 创建",
	"profile.list_empty_filter": "没有匹配 %q 的配置",
	"field.required_host":       "主机为必填项",
	"field.required_user":       "用户为必填项",
	"field.edit_header":         "正在编辑配置 %q（回车保持原值）",
	"auth.unsupported":          "不支持的认证方式 %q",
	"add.required_password":     "password auth 需要提供 --password",
	"add.required_keypath":      "key auth 需要提供 --key-path",
	"password.read_error":       "读取密码失败: %v",
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

	// Interactive prompts
	"prompt.auth_method":       "认证方式",
	"prompt.host":              "主机",
	"prompt.port":              "端口",
	"prompt.user":              "用户",
	"prompt.key_path":          "密钥路径",
	"prompt.key_passphrase":    "密钥密码",
	"prompt.key_has_pass":      "密钥有密码？",
	"prompt.proxy":             "跳板机配置（可留空）",
	"prompt.options":           "选项（逗号分隔 KEY=VALUE，可留空）",
	"prompt.tags":              "标签（逗号分隔，可留空）",
	"prompt.save":              "保存配置？",
	"prompt.save_changes":      "保存修改？",
	"prompt.discard":           "放弃所有修改？",
	"prompt.remove_proxy":      "移除跳板？",
	"prompt.remove_options":    "移除所有选项？",
	"prompt.change_password":   "更改密码？",
	"prompt.new_password":      "新密码",
	"prompt.select":            "选择",
	"prompt.choose":            "选择",
	"prompt.action":            "操作",
	"prompt.value":             "值",
	"prompt.key":               "键",
	"prompt.unset_key":         "删除哪个键？",
	"prompt.unset_confirm":     "删除此键？",
	"prompt.config_panel":      "QSSH 配置",
	"prompt.available_keys":    "可用键",
	"prompt.no_keys":           "没有可删除的键",
	"prompt.preview":           "预览",
	"prompt.creating":          "创建配置",
	"prompt.editing":           "正在编辑",
	"prompt.edit_menu.":        "主机/端口/用户",
	"prompt.edit_menu.1":       "认证方式与凭据",
	"prompt.edit_menu.2":       "跳板机",
	"prompt.edit_menu.3":       "选项",
	"prompt.edit_menu.4":       "标签",
	"prompt.edit_menu.5":       "保存并退出",
	"prompt.edit_menu.6":       "放弃",
	"prompt.set":               "设置键",
	"prompt.unset":             "删除键",
	"prompt.quit":              "退出",
	"prompt.invalid":           "无效选择",
	"prompt.password_mismatch": "两次密码不一致",
	"prompt.key_not_found":     "警告：密钥文件 %q 未找到",
	"prompt.proxy_self":        "错误：跳板不能指向同一配置",
	"prompt.proxy_not_found":   "警告：跳板配置 %q 未找到",

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
