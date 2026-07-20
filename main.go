package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"qssh/cmd"
	"qssh/internal"
	"qssh/internal/i18n"
	"qssh/internal/privacy"
)

var version = "dev"

// privacyFlagValue accepts bare --privacy (status) or --privacy=on|off|clear.
type privacyFlagValue struct {
	present bool
	value   string
}

func (p *privacyFlagValue) String() string { return p.value }
func (p *privacyFlagValue) Set(s string) error {
	p.present = true
	p.value = s
	return nil
}

func main() {
	// Load config and apply locale override before any command runs.
	if cfg := internal.OpenConfig(internal.DefaultConfigPath()); cfg != nil {
		if lang := cfg.Get("lang"); lang != "" {
			i18n.SetLocale(lang)
		}
	}

	var (
		addName         string
		addHost         string
		addPort         int
		addUser         string
		addAuth         string
		addPassword     string
		addKeyPath      string
		addKeyPass      string
		addProxy        string
		addOptionsStr   string
		addTagsStr      string
		editName        string
		delName         string
		copyOld         string
		renameOld       string
		historyProfile  string
		historyLast     bool
		sftpStartName   string
		sftpBind        string
		sftpAllowRemote bool
		sftpStopName    string
		execName        string
		daemonStart     string // --daemon-start
		daemonStop      string // --daemon-stop
		sftpDaemon      string // --sftp-daemon (internal)
		daemonRunName   string // --daemon-run (internal)
		daemonModeFlag  string // --daemon-mode (internal)
		daemonPort      string // --port (internal)
		daemonBind      string // --bind-addr (internal)
		doConfig        bool
		doList          bool
		listJSON        bool
		forceYes        bool
		showVer         bool
		reveal          bool
		privFlag        privacyFlagValue
	)

	flag.StringVar(&addName, "add", "", "Create a new profile")
	flag.StringVar(&addHost, "host", "", "Host for --add")
	flag.IntVar(&addPort, "port", 0, "Port for --add")
	flag.StringVar(&addUser, "user", "", "User for --add")
	flag.StringVar(&addAuth, "auth", "", "Auth method for --add (password/key/agent/keyboard-interactive)")
	flag.StringVar(&addPassword, "password", "", "Password for --add")
	flag.StringVar(&addKeyPath, "key-path", "", "Key path for --add (used with --auth key)")
	flag.StringVar(&addKeyPass, "key-passphrase", "", "Passphrase for encrypted private key (--add/--edit)")
	flag.StringVar(&addProxy, "proxy", "", "Proxy profile name for --add or --edit")
	flag.StringVar(&addOptionsStr, "set-option", "", "Options for --add (comma-separated KEY=VALUE pairs, e.g. ConnectTimeout=30s,SetEnv=LANG=en_US.UTF-8)")
	flag.StringVar(&addTagsStr, "tags", "", "Comma-separated tags for --add or --edit")
	flag.StringVar(&editName, "edit", "", "Edit an existing profile")
	flag.StringVar(&delName, "delete", "", "Delete a profile")
	flag.StringVar(&copyOld, "copy", "", "Copy a profile (usage: qssh --copy <old-name> <new-name>)")
	flag.StringVar(&renameOld, "rename", "", "Rename a profile (usage: qssh --rename <old-name> <new-name>)")
	flag.StringVar(&historyProfile, "history", "", "Show connection history for a profile")
	flag.BoolVar(&historyLast, "last", false, "Show only the last connection (use with --history)")
	flag.StringVar(&sftpStartName, "sftp-start", "", "Start SFTP proxy for a profile (usage: qssh --sftp-start <name>)")
	flag.StringVar(&sftpBind, "bind", "", "Bind address for SFTP proxy (default: 127.0.0.1)")
	flag.BoolVar(&sftpAllowRemote, "sftp-allow-remote", false, "Allow SFTP proxy to bind non-loopback addresses (dangerous)")
	flag.StringVar(&sftpStopName, "sftp-stop", "", "Stop SFTP proxy for a profile (usage: qssh --sftp-stop <name>)")
	flag.StringVar(&execName, "exec", "", "Run a command on a profile (usage: qssh --exec <profile> <command>)")
	flag.StringVar(&daemonStart, "daemon-start", "", "Start background daemon for connection reuse")
	flag.StringVar(&daemonStop, "daemon-stop", "", "Stop a background daemon")
	flag.StringVar(&sftpDaemon, "sftp-daemon", "", "Internal: SFTP proxy worker (profile name)")
	flag.StringVar(&daemonRunName, "daemon-run", "", "Internal: daemon worker")
	flag.StringVar(&daemonModeFlag, "daemon-mode", "", "Internal: daemon mode (persistent|managed)")
	flag.StringVar(&daemonPort, "daemon-port", "", "Internal: port")
	flag.StringVar(&daemonBind, "bind-addr", "", "Internal: bind address")
	flag.BoolVar(&doConfig, "config", false, "View or modify config (usage: qssh --config [get|set <key> <value>])")
	flag.BoolVar(&doList, "list", false, "List profiles (optional: qssh --list filter)")
	flag.BoolVar(&listJSON, "json", false, "Machine-readable JSON output (use with --list)")
	flag.BoolVar(&forceYes, "yes", false, "Skip confirmation prompts (agent-friendly)")
	flag.BoolVar(&forceYes, "y", false, "Short for --yes")
	flag.BoolVar(&showVer, "version", false, "Print version")
	flag.BoolVar(&reveal, "reveal", false, "Show hosts/IPs for this process only (does not change sticky privacy)")
	flag.Var(&privFlag, "privacy", "Privacy mode: on|off|clear|status (sticky until reboot; bare --privacy = status)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, i18n.T("usage.text"), version)
	}
	os.Args = normalizePrivacyArgs(os.Args)
	flag.Parse()

	// --reveal is process-local and takes effect before any command output.
	if reveal {
		privacy.RevealOnce()
	}

	optsMap := parseOptionsString(addOptionsStr)

	switch {
	case showVer:
		fmt.Printf("qssh %s\n", version)
		return
	case privFlag.present:
		args := []string{}
		switch strings.ToLower(privFlag.value) {
		case "", "status":
			// print status only
		default:
			args = []string{privFlag.value}
		}
		cmd.Privacy(args)
		return
	case daemonRunName != "":
		mode := "persistent"
		if daemonModeFlag == "managed" {
			mode = "managed"
		}
		cmd.RunDaemon(daemonRunName, mode)
	case daemonStart != "":
		cmd.StartDaemon(daemonStart)
	case daemonStop != "":
		cmd.StopDaemon(daemonStop)
	case sftpDaemon != "":
		cmd.SftpDaemon(sftpDaemon, daemonPort, daemonBind, sftpAllowRemote)
	case doConfig:
		cmd.Config(flag.Args())
	case addName != "":
		cmd.Add(cmd.AddOpts{
			Name:          addName,
			Host:          addHost,
			Port:          addPort,
			User:          addUser,
			Auth:          addAuth,
			Password:      addPassword,
			KeyPath:       addKeyPath,
			KeyPassphrase: addKeyPass,
			Proxy:         addProxy,
			Options:       optsMap,
			Tags:          parseTags(addTagsStr),
		})
	case editName != "":
		editOpts := cmd.AddOpts{
			Host:          addHost,
			Port:          addPort,
			User:          addUser,
			Auth:          addAuth,
			Password:      addPassword,
			KeyPath:       addKeyPath,
			KeyPassphrase: addKeyPass,
			Proxy:         addProxy,
			Options:       optsMap,
			Tags:          parseTags(addTagsStr),
		}
		cmd.Edit(editName, editOpts)
	case delName != "":
		cmd.Delete(delName, forceYes)
	case copyOld != "":
		newName := flag.Arg(0)
		if newName == "" {
			fmt.Fprintln(os.Stderr, "usage: qssh --copy <old-name> <new-name>")
			os.Exit(1)
		}
		cmd.Copy(copyOld, newName)
	case renameOld != "":
		newName := flag.Arg(0)
		if newName == "" {
			fmt.Fprintln(os.Stderr, "usage: qssh --rename <old-name> <new-name>")
			os.Exit(1)
		}
		cmd.Rename(renameOld, newName)
	case historyProfile != "" || historyLast:
		cmd.History(historyProfile, historyLast)
	case execName != "":
		if flag.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "error: --exec requires a command")
			os.Exit(1)
		}
		// Pass raw argv so spaces/quotes survive remote shell quoting.
		cmd.Exec(execName, flag.Args())
	case sftpStartName != "":
		bindAddr := sftpBind
		allowRemote := sftpAllowRemote
		if cfg := internal.OpenConfig(internal.DefaultConfigPath()); cfg != nil {
			if bindAddr == "" {
				bindAddr = cfg.Get("sftp.bind")
			}
			// Only widen allowRemote from config when the file is healthy.
			// A corrupt config must not silently enable remote listening.
			if !allowRemote && cfg.LoadError() == nil {
				v := strings.ToLower(strings.TrimSpace(cfg.Get("sftp.allow_non_loopback")))
				allowRemote = v == "true" || v == "1" || v == "yes"
			}
		}
		if bindAddr == "" {
			bindAddr = "127.0.0.1"
		}
		cmd.SftpStart(sftpStartName, bindAddr, addPort, allowRemote)
	case sftpStopName != "":
		cmd.SftpStop(sftpStopName)
	case doList || listJSON:
		filter := ""
		if flag.NArg() > 0 {
			filter = flag.Arg(0)
		}
		cmd.List(filter, listJSON)
	case flag.NArg() == 1:
		connectCmd(flag.Arg(0))
	default:
		flag.Usage()
		os.Exit(1)
	}
}

// normalizePrivacyArgs rewrites:
//
//	--privacy           -> --privacy=status
//	--privacy on|off... -> --privacy=on|off...
//
// so flag.Var always receives a value (works with space-separated form).
func normalizePrivacyArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--privacy" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				out = append(out, "--privacy="+args[i+1])
				i++
				continue
			}
			out = append(out, "--privacy=status")
			continue
		}
		out = append(out, a)
	}
	return out
}

func connectCmd(name string) { cmd.Connect(name) }

// parseOptionsString parses a comma-separated KEY=VALUE string into a map.
func parseOptionsString(s string) map[string]string {
	if s == "" {
		return nil
	}
	m := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			fmt.Fprintf(os.Stderr, "warning: ignoring malformed option %q (expected KEY=VALUE)\n", pair)
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if key != "" {
			m[key] = val
		}
	}
	return m
}

// parseTags splits a comma-separated tag list.
func parseTags(s string) []string {
	if s == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
