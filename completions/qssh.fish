# fish completion for qssh
# source: source completions/qssh.fish
# install: cp completions/qssh.fish ~/.config/fish/completions/

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

complete -c qssh -l add -d "Create a new profile" -r
complete -c qssh -l edit -d "Edit an existing profile" -r -a "(__qssh_profiles)"
complete -c qssh -l delete -d "Delete a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l list -d "List profiles"
complete -c qssh -l json -d "JSON output (with --list)"
complete -c qssh -l yes -d "Skip confirmation prompts"
complete -c qssh -s y -d "Short for --yes"
complete -c qssh -l exec -d "Execute a command on a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l sftp-start -d "Start SFTP proxy" -r -a "(__qssh_profiles)"
complete -c qssh -l sftp-stop -d "Stop SFTP proxy" -r -a "(__qssh_profiles)"
complete -c qssh -l daemon-start -d "Start background daemon" -r -a "(__qssh_profiles)"
complete -c qssh -l daemon-stop -d "Stop background daemon" -r -a "(__qssh_profiles)"
complete -c qssh -l config -d "View or modify config"
complete -c qssh -l version -d "Print version"

complete -c qssh -l host -d "Host for --add or --edit" -r
complete -c qssh -l port -d "Port (for --add, --edit, or --sftp-start)" -r
complete -c qssh -l user -d "User for --add or --edit" -r
complete -c qssh -l auth -d "Auth method (password/key/agent)" -r -xa "password key agent keyboard-interactive"
complete -c qssh -l password -d "Password for --add or --edit" -r
complete -c qssh -l key-path -d "Key path for --add or --edit" -r
complete -c qssh -l key-passphrase -d "Passphrase for encrypted private key" -r
complete -c qssh -l tags -d "Comma-separated tags" -r
complete -c qssh -l proxy -d "Proxy profile for --add or --edit" -r -a "(__qssh_profiles)"
complete -c qssh -l set-option -d "Options for --add (KEY=VALUE,KEY2=VALUE2)" -r
complete -c qssh -l bind -d "Bind address for --sftp-start" -r

complete -c qssh -l copy -d "Copy a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l rename -d "Rename a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l history -d "Show connection history" -r -a "(__qssh_profiles)"
complete -c qssh -l last -d "Show only last connection"
