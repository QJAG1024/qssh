# fish completion for qssh
# source: source completions/qssh.fish
# install: cp completions/qssh.fish ~/.config/fish/completions/

complete -c qssh -f

# Commands
complete -c qssh -l add -d "Create a new profile" -r
complete -c qssh -l edit -d "Edit an existing profile" -r
complete -c qssh -l delete -d "Delete a profile" -r
complete -c qssh -l list -d "List profiles"
complete -c qssh -l exec -d "Execute a command on a profile" -r
complete -c qssh -l sftp-start -d "Start SFTP proxy" -r
complete -c qssh -l sftp-stop -d "Stop SFTP proxy" -r
complete -c qssh -l daemon-start -d "Start background daemon" -r
complete -c qssh -l daemon-stop -d "Stop background daemon" -r
complete -c qssh -l config -d "View or modify config"
complete -c qssh -l version -d "Print version"

# Add flags
complete -c qssh -l host -d "Host for --add" -r
complete -c qssh -l port -d "Port (for --add or --sftp-start)" -r
complete -c qssh -l user -d "User for --add" -r
complete -c qssh -l auth -d "Auth method (password/key/agent)" -r -xa "password key agent keyboard-interactive"
complete -c qssh -l password -d "Password for --add" -r
complete -c qssh -l key-path -d "Key path for --add" -r
complete -c qssh -l bind -d "Bind address for --sftp-start" -r