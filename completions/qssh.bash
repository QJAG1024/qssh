# bash completion for qssh
_qssh() {
  local cur prev words cword
  _init_completion || return

  local profiles
  profiles=$(qssh --list 2>/dev/null | tail -n +3 | cut -f1 | tr '\n' ' ')

  case $prev in
    --edit|--delete|--exec|--sftp-start|--sftp-stop|--daemon-start|--daemon-stop)
      COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
      return ;;
    --copy|--rename)
      COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
      return ;;
    --host|--user|--auth|--password|--key-path|--set-option|--bind|--proxy)
      return ;;
    --add|--list|--history)
      return ;;
  esac

  if [[ $cur == -* ]]; then
    COMPREPLY=($(compgen -W '--add --edit --delete --list --exec --sftp-start --sftp-stop --daemon-start --daemon-stop --copy --rename --history --config --version --host --port --user --auth --password --key-path --proxy --set-option --bind --last' -- "$cur"))
  fi
}
complete -F _qssh qssh