# fish completion for qssh
# source: source completions/qssh.fish
# install: cp completions/qssh.fish ~/.config/fish/completions/

function __qssh_profiles
    qssh --list 2>/dev/null | string match -r '^[^\t]+' | tail -n +3 | string trim
end

complete -c qssh -f

# Commands
complete -c qssh -l add -d "Create a new profile" -r
complete -c qssh -l edit -d "Edit an existing profile" -r -a "(__qssh_profiles)"
complete -c qssh -l delete -d "Delete a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l list -d "List profiles"
complete -c qssh -l exec -d "Execute a command on a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l sftp-start -d "Start SFTP proxy" -r -a "(__qssh_profiles)"
complete -c qssh -l sftp-stop -d "Stop SFTP proxy" -r -a "(__qssh_profiles)"
complete -c qssh -l daemon-start -d "Start background daemon" -r -a "(__qssh_profiles)"
complete -c qssh -l daemon-stop -d "Stop background daemon" -r -a "(__qssh_profiles)"
complete -c qssh -l config -d "View or modify config"
complete -c qssh -l version -d "Print version"

# Add/Edit flags
complete -c qssh -l host -d "Host for --add or --edit" -r
complete -c qssh -l port -d "Port (for --add, --edit, or --sftp-start)" -r
complete -c qssh -l user -d "User for --add or --edit" -r
complete -c qssh -l auth -d "Auth method (password/key/agent)" -r -xa "password key agent keyboard-interactive"
complete -c qssh -l password -d "Password for --add or --edit" -r
complete -c qssh -l key-path -d "Key path for --add or --edit" -r
complete -c qssh -l proxy -d "Proxy profile for --add or --edit" -r -a "(__qssh_profiles)"
complete -c qssh -l set-option -d "Options for --add (KEY=VALUE,KEY2=VALUE2)" -r
complete -c qssh -l bind -d "Bind address for --sftp-start" -r

# Copy/Rename/History
complete -c qssh -l copy -d "Copy a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l rename -d "Rename a profile" -r -a "(__qssh_profiles)"
complete -c qssh -l history -d "Show connection history" -r -a "(__qssh_profiles)"
complete -c qssh -l last -d "Show only last connection"