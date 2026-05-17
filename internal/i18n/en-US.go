package i18n

var enUS = map[string]string{
	// Meta
	"locale.code": "en-US",

	// Step labels (used in progress output)
	"step.decrypt":          "Decrypt credentials",
	"step.dns_resolve":      "DNS resolve",
	"step.tcp_connect":      "TCP connect",
	"step.ssh_handshake":    "SSH handshake",
	"step.authenticate":     "Authenticate",
	"step.allocate_pty":     "Allocate PTY",
	"step.shell_start":      "Start shell",
	"step.unknown":          "Unknown step",

	// Session progress messages
	"profile.loaded":        "Profile loaded",
	"resolving":             "Resolving %s",
	"dns_resolve.failed":    "DNS resolution failed: %v",
	"dns_resolve.hint":      "Check the hostname or IP address in the profile",
	"dns_resolve.detail":    "%s → %s (%dms)",
	"connecting":            "Connecting to %s",
	"tcp_connect.failed":    "TCP connection failed: %s",
	"tcp_connect.hint":      "Confirm the host is online, port is correct, and firewall allows access",
	"authenticate.failed":   "Authentication failed: %v",
	"authenticate.hint":     "Check credentials in profile: qssh --edit %s",
	"connected":             "Connected in %dms",
	"pty_allocate.failed":   "PTY allocation failed: %v",
	"shell_start.failed":    "Shell start failed: %v",
	"session.ready":         "Session established, entering interactive mode",

	// Profile CRUD
	"store.open_error":      "Error opening store: %v",
	"profile.not_found":     "Profile %q not found.",
	"profile.exists":        "Profile %q already exists. Use 'qssh --edit' to modify it.",
	"profile.created":       "Profile %q created. Use 'qssh %s' to connect.",
	"profile.updated":       "Profile %q updated.",
	"profile.deleted":       "Profile %q deleted.",
	"profile.delete_confirm":"Delete profile %q?",
	"profile.cancelled":     "Cancelled.",
	"profile.list_empty":    "No profiles. Use 'qssh --add <name>' to create one.",
	"profile.list_empty_filter": "No profiles matching %q.",
	"field.required_host":   "Host is required.",
	"field.required_user":   "User is required.",
	"field.edit_header":     "Editing profile %q (press Enter to keep current value)",
	"auth.unsupported":      "Unsupported auth method %q",
	"password.read_error":   "Error reading password: %v",
	"password.change_prompt":"Change password?",
	"password.new_prompt":   "New password",
	"profile.save_error":    "Error saving profile: %v",

	// Connection
	"connect.failed":        "Connection failed.",
	"connect.ended":         "Session ended: %v",
	"profile.header":        "Profile: %s (%s@%s:%d)",
	"session.closed":        "  ⚡ Connection closed (%s)",

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

	// Mount
	"mount.mounted":         "Mounted: %s",
	"mount.failed":          "Mount failed: %v",
	"mount.unmount_failed":  "Unmount failed: %v",
	"mount.daemon_failed":   "daemon failed",
	"mount.backend_failed":  "%s mount failed: %s",
	"mount.unmounted":       "Unmounted",
	"mount.backend_not_found": "%s not found",

	// List table headers
	"list.header.name":      "Name",
	"list.header.host":      "Host",
	"list.header.port":      "Port",
	"list.header.user":      "User",
	"list.header.auth":      "Auth",
	"list.header.last_used": "Last Used",
	"list.header.count":     "Count",

	// Time
	"time.just_now":         "just now",
	"time.minutes_ago":      "%dm ago",
	"time.hours_ago":        "%dh ago",

	// Usage
	"usage.text": `QSSH - SSH Credential Manager v%s

Usage:
  qssh <profile>                    Connect to a profile
  qssh --add <name>                 Create a new profile
  qssh --edit <name>                Edit an existing profile
  qssh --list [filter]              List profiles (optional substring filter)
  qssh --delete <name>              Delete a profile
  qssh --mount <name>               Mount profile via WebDAV (background)
  qssh --umount <name>              Unmount a profile
  qssh --config [get|set ...]       View or modify config
  qssh --version                    Print version`,
}