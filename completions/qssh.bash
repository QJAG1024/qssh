# bash completion for qssh
_qssh() {
  local cur prev words cword
  _init_completion || return

  local profiles
  profiles=$(qssh --list --json 2>/dev/null | python3 -c 'import json,sys
try:
  data=json.load(sys.stdin)
  print(" ".join(p.get("name","") for p in data))
except Exception:
  pass' 2>/dev/null)
  if [[ -z $profiles ]]; then
    profiles=$(qssh --list 2>/dev/null | tail -n +3 | awk 'NF{print $1}' | tr '\n' ' ')
  fi

  case $prev in
    --edit|--delete|--exec|--sftp-start|--sftp-stop|--daemon-start|--daemon-stop|--history|--proxy)
      COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
      return ;;
    --copy|--rename)
      COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
      return ;;
    --host|--user|--auth|--password|--key-path|--key-passphrase|--set-option|--bind|--tags|--port)
      return ;;
    --add|--list)
      return ;;
  esac

  if [[ $cur == -* ]]; then
    COMPREPLY=($(compgen -W '--add --edit --delete --list --json --yes -y --exec --sftp-start --sftp-stop --daemon-start --daemon-stop --copy --rename --history --config --version --host --port --user --auth --password --key-path --key-passphrase --proxy --set-option --tags --bind --last' -- "$cur"))
  else
    COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
  fi
}
complete -F _qssh qssh
