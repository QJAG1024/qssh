# fish completion for qssh
function __qssh_profiles
    if command qssh --list --json >/dev/null 2>&1
        command qssh --list --json 2>/dev/null | command python3 -c 'import json,sys
try:
  data=json.load(sys.stdin)
  for p in data: print(p.get("name",""))
except Exception:
  pass' 2>/dev/null
    else
        command qssh --list 2>/dev/null | tail -n +3 | awk 'NF{print $1}'
    end
end

complete -c qssh -f

complete -c qssh -l add -d "Create a new profile" -r -a "(__qssh_profiles)"
complete -c qssh -l auth -d "Auth method for --add (password/key/agent/keyboard-interactive)" -r
complete -c qssh -l bind -d "Bind address for SFTP proxy (default: 127.0.0.1)" -r
complete -c qssh -l config -d "View or modify config (usage: qssh --config [get|set <key> <value>])"
complete -c qssh -l copy -d "Copy a profile (usage: qssh --copy <old-name> <new-name>)" -r -a "(__qssh_profiles)"
complete -c qssh -l daemon-start -d "Start background daemon for connection reuse" -r -a "(__qssh_profiles)"
complete -c qssh -l daemon-stop -d "Stop a background daemon" -r -a "(__qssh_profiles)"
complete -c qssh -l delete -d "Delete a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l dir -d "Output dir for --export (default: current dir; non-interactive mode reads passphrase from stdin)" -r
complete -c qssh -l edit -d "Edit an existing profile" -r -a "(__qssh_profiles)"
complete -c qssh -l exec -d "Run a command on a profile (usage: qssh --exec <profile> <command>)" -r -a "(__qssh_profiles)"
complete -c qssh -l export -d "Export a profile to an encrypted .qssh file (usage: qssh --export <name>)" -r -a "(__qssh_profiles)"
complete -c qssh -l history -d "Show connection history for a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l host -d "Host for --add" -r
complete -c qssh -l import -d "Import a profile from an encrypted .qssh file (usage: qssh --import <file>)" -r
complete -c qssh -l json -d "Machine-readable JSON output (use with --list)"
complete -c qssh -l key-passphrase -d "Passphrase for encrypted private key (--add/--edit)" -r
complete -c qssh -l key-path -d "Key path for --add (used with --auth key)" -r
complete -c qssh -l last -d "Show only the last connection (use with --history)"
complete -c qssh -l list -d "List profiles (optional: qssh --list filter)"
complete -c qssh -l name -d "Profile name for --import (non-interactive mode reads passphrase from stdin)" -r
complete -c qssh -l password -d "Password for --add" -r
complete -c qssh -l port -d "Port for --add" -r
complete -c qssh -l proxy -d "Proxy profile name for --add or --edit" -r -a "(__qssh_profiles)"
complete -c qssh -l rename -d "Rename a profile (usage: qssh --rename <old-name> <new-name>)" -r -a "(__qssh_profiles)"
complete -c qssh -l reveal -d "Show hosts/IPs for this process only (does not change sticky privacy)"
complete -c qssh -l set-option -d "Options for --add (comma-separated KEY=VALUE pairs, e.g. ConnectTimeout=30s,SetEnv=LANG=en_US.UTF-8)" -r
complete -c qssh -l sftp -d "Show SFTP proxy status (usage: qssh --sftp [profile])"
complete -c qssh -l sftp-allow-remote -d "DEPRECATED: non-loopback binds are now authorized by --bind or per-profile sftp.bind"
complete -c qssh -l sftp-start -d "Start SFTP proxy for a profile (usage: qssh --sftp-start <name>)" -r -a "(__qssh_profiles)"
complete -c qssh -l sftp-stop -d "Stop SFTP proxy for a profile (usage: qssh --sftp-stop <name>)" -r -a "(__qssh_profiles)"
complete -c qssh -l tags -d "Comma-separated tags for --add or --edit" -r
complete -c qssh -l user -d "User for --add" -r
complete -c qssh -l version -d "Print version"
complete -c qssh -l webdav -d "Show WebDAV mount status (usage: qssh --webdav [profile])"
complete -c qssh -l webdav-start -d "Start WebDAV server for a profile (usage: qssh --webdav-start <name>)" -r -a "(__qssh_profiles)"
complete -c qssh -l webdav-stop -d "Stop WebDAV server for a profile" -r -a "(__qssh_profiles)"
complete -c qssh -s y -d "Short for --yes"
complete -c qssh -l yes -d "Skip confirmation prompts (agent-friendly)"
