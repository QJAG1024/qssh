package i18n

var enUS = map[string]string{
	// Meta
	"locale.code": "en-US",

	// Step labels (used in progress output)
	"step.decrypt":       "Decrypt credentials",
	"step.dns_resolve":   "DNS resolve",
	"step.tcp_connect":   "TCP connect",
	"step.ssh_handshake": "SSH handshake",
	"step.authenticate":  "Authenticate",
	"step.allocate_pty":  "Allocate PTY",
	"step.shell_start":   "Start shell",
	"step.proxy_connect": "Proxy connect",
	"step.unknown":       "Unknown step",

	// Session progress messages
	"profile.loaded":      "Profile loaded",
	"resolving":           "Resolving %s",
	"dns_resolve.failed":  "DNS resolution failed: %v",
	"dns_resolve.hint":    "Check the hostname or IP address in the profile",
	"dns_resolve.detail":  "%s → %s (%dms)",
	"connecting":          "Connecting to %s",
	"tcp_connect.failed":  "TCP connection failed: %s",
	"tcp_connect.hint":    "Confirm the host is online, port is correct, and firewall allows access",
	"authenticate.failed": "Authentication failed: %v",
	"authenticate.hint":   "Check credentials in profile: qssh --edit %s",
	"connected":           "Connected in %dms",
	"pty_allocate.failed": "PTY allocation failed: %v",
	"shell_start.failed":  "Shell start failed: %v",
	"session.ready":       "Session established, entering interactive mode",

	// Profile CRUD
	"store.open_error":          "Error opening store: %v",
	"profile.not_found":         "Profile %q not found.",
	"profile.exists":            "Profile %q already exists. Use 'qssh --edit' to modify it.",
	"profile.created":           "Profile %q created. Use 'qssh %s' to connect.",
	"profile.updated":           "Profile %q updated.",
	"profile.deleted":           "Profile %q deleted.",
	"profile.delete_confirm":    "Delete profile %q?",
	"profile.cancelled":         "Cancelled.",
	"profile.list_empty":        "No profiles. Use 'qssh --add <name>' to create one.",
	"profile.list_empty_filter": "No profiles matching %q.",
	"field.required_host":       "Host is required.",
	"field.required_user":       "User is required.",
	"field.edit_header":         "Editing profile %q (press Enter to keep current value)",
	"auth.unsupported":          "Unsupported auth method %q",
	"add.required_password":     "password is required for password auth",
	"add.required_keypath":      "--key-path is required for key auth",
	"password.read_error":       "Error reading password: %v",
	"password.change_prompt":    "Change password?",
	"password.new_prompt":       "New password",
	"profile.save_error":        "Error saving profile: %v",

	// Proxy / Jump host
	"proxy.connecting": "Connecting to jump host %s...",
	"proxy.tunneling":  "Tunneling through %s to %s",
	"proxy.handshake":  "SSH handshake with target via %s",

	// Connection history
	"history.header":     "Connection History for %s",
	"history.header_all": "Connection History",
	"history.time":       "Time",
	"history.duration":   "Duration",
	"history.command":    "Command",
	"history.exit":       "Exit Code",
	"history.empty":      "No history found.",
	"history.empty_all":  "No connection history found.",

	// Connection
	"connect.failed":         "Connection failed.",
	"connect.ended":          "Session ended: %v",
	"profile.header":         "Profile: %s (%s@%s:%d)",
	"profile.header_private": "Profile: %s (%s)",
	"session.closed":         "  ⚡ Connection closed (%s)",
	"privacy.status":         "privacy: %s (source: %s)",
	"privacy.set":            "privacy sticky set to %s (until reboot)",
	"privacy.cleared":        "privacy sticky cleared (default: on)",
	"privacy.usage":          "Usage: qssh --privacy [on|off|clear|status]",

	// Config
	"config.usage.get":      "Usage: qssh --config get <key>",
	"config.usage.set":      "Usage: qssh --config set <key> <value>",
	"config.usage.unset":    "Usage: qssh --config unset <key>",
	"config.unknown_action": "Unknown config action %q (use get/set/unset)",
	"config.empty":          "(no config)",
	"config.not_set":        "(not set)",
	"config.set":            "%s = %s",
	"config.unset":          "%s unset",
	"config.save_error":     "Error saving config: %v",

	// SFTP
	"sftp.preparing":      "Starting...",
	"sftp.opening_store":  "Opening store...",
	"sftp.connecting":     "Connecting SSH...",
	"sftp.starting":       "Starting SFTP...",
	"sftp.starting_proxy": "Starting SFTP proxy...",
	"sftp.proxy_started":  "SFTP proxy: %s",
	"sftp.failed":         "SFTP start failed: %v",
	"sftp.stop_failed":    "SFTP stop failed: %v",
	"sftp.stopped":        "SFTP stopped",
	"sftp.daemon_failed":  "daemon failed",

	// SFTP bind authorization
	"sftp.bind.warn_cli":        "Warning: SFTP proxy binds to %s (non-loopback). The proxy accepts any password — the remote server's file system will be reachable from the network. Proceeding in 2s...",
	"sftp.bind.deprecated_flag": "Warning: --sftp-allow-remote is deprecated and no longer needed; non-loopback binds are authorized by --bind or per-profile sftp.bind.",
	"sftp.bind.refuse_global":   "refusing to start: global sftp.bind=%s is non-loopback but sftp.allow_non_loopback is not true.",
	"sftp.bind.refuse_hint":     "If this profile should listen on a non-loopback address, set sftp.bind on the profile itself (per-profile choice authorizes it). Otherwise set sftp.allow_non_loopback=true to accept the risk globally.",
	"sftp.bind.set_warn":        "Warning: global sftp.bind is non-loopback. qssh will refuse to start such binds unless sftp.allow_non_loopback=true.",
	"sftp.bind.set_hint_allow":  "If you understand the risk, run: qssh --config set sftp.allow_non_loopback true",
	"sftp.bind.set_hint_profile": "Tip: set sftp.bind on a single profile instead (per-profile choice authorizes it): qssh --edit <profile> --set-option sftp.bind=%s",

	// Config interactive panel
	"config.panel.title":      "QSSH Configuration",
	"config.panel.not_set":    "(not set)",
	"config.panel.set":        "s) set a key",
	"config.panel.unset":      "u) unset a key",
	"config.panel.quit":       "q) quit",
	"config.panel.action":     "Action",
	"config.panel.key":        "Key",
	"config.panel.value":      "Value",
	"config.panel.unset_which": "Unset which key?",
	"config.panel.unset_confirm": "Unset this key?",
	"config.panel.remove_all":   "Remove all options?",

	// List table headers
	"list.header.name":      "Name",
	"list.header.host":      "Host",
	"list.header.port":      "Port",
	"list.header.user":      "User",
	"list.header.auth":      "Auth",
	"list.header.last_used": "Last Used",
	"list.header.count":     "Count",
	"list.header.proxy":     "Proxy",

	// Time
	"time.just_now":    "just now",
	"time.minutes_ago": "%dm ago",
	"time.hours_ago":   "%dh ago",

	// Interactive prompts
	"prompt.auth_method":       "Auth method",
	"prompt.host":              "Host",
	"prompt.port":              "Port",
	"prompt.user":              "User",
	"prompt.key_path":          "Key path",
	"prompt.key_passphrase":    "Key passphrase",
	"prompt.key_has_pass":      "Key has passphrase?",
	"prompt.proxy":             "ProxyJump profile (optional)",
	"prompt.options":           "Options (comma-separated KEY=VALUE, optional)",
	"prompt.tags":              "Tags (comma-separated, optional)",
	"prompt.save":              "Save profile?",
	"prompt.save_changes":      "Save changes?",
	"prompt.discard":           "Discard all changes?",
	"prompt.remove_proxy":      "Remove proxy?",
	"prompt.remove_options":    "Remove all options?",
	"prompt.change_password":   "Change password?",
	"prompt.new_password":      "New password",
	"prompt.select":            "Select",
	"prompt.choose":            "Choose",
	"prompt.action":            "Action",
	"prompt.value":             "Value",
	"prompt.key":               "Key",
	"prompt.unset_key":         "Unset which key?",
	"prompt.unset_confirm":     "Unset this key?",
	"prompt.config_panel":      "QSSH Configuration",
	"prompt.available_keys":    "Available keys",
	"prompt.no_keys":           "No keys to unset",
	"prompt.preview":           "Preview",
	"prompt.creating":          "Creating profile",
	"prompt.editing":           "Editing",
	"prompt.edit_menu.":        "Host/Port/User",
	"prompt.edit_menu.1":       "Auth method & credentials",
	"prompt.edit_menu.2":       "ProxyJump",
	"prompt.edit_menu.3":       "Options",
	"prompt.edit_menu.4":       "Tags",
	"prompt.edit_menu.5":       "Save & exit",
	"prompt.edit_menu.6":       "Discard",
	"prompt.set":               "set a key",
	"prompt.unset":             "unset a key",
	"prompt.quit":              "quit",
	"prompt.invalid":           "Invalid choice",
	"prompt.password_mismatch": "Passwords do not match",
	"prompt.key_not_found":     "Warning: key file %q not found",
	"prompt.proxy_self":        "Error: proxy cannot point to the same profile",
	"prompt.proxy_not_found":   "Warning: proxy profile %q not found",

	// Usage
	"usage.text": `QSSH - SSH Credential Manager v%s

Usage:
  qssh <profile>                    Connect to a profile
  qssh --add <name>                 Create a new profile
  qssh --edit <name>                Edit an existing profile
  qssh --list [filter]              List profiles (optional substring filter)
  qssh --copy <old> <new>           Copy a profile
  qssh --rename <old> <new>         Rename a profile
  qssh --delete <name>              Delete a profile
  qssh --exec <profile> <command>   Execute a command on a profile
  qssh --sftp-start <name>          Start SFTP proxy for a profile
  qssh --sftp-stop <name>           Stop SFTP proxy for a profile
  qssh --daemon-start <name>        Start background daemon
  qssh --daemon-stop <name>         Stop background daemon
  qssh --history [name]             Show connection history
  qssh --config [get|set ...]       View or modify config
  qssh --version                    Print version`,
}
