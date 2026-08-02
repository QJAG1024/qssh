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
    --add|--copy|--daemon-start|--daemon-stop|--delete|--edit|--exec|--export|--history|--proxy|--rename|--sftp-start|--sftp-stop|--webdav-start|--webdav-stop)
      COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
      return ;;
  esac

  if [[ $cur == -* ]]; then
    COMPREPLY=($(compgen -W '--add --auth --bind --config --copy --daemon-start --daemon-stop --delete --dir --edit --exec --export --history --host --import --json --key-passphrase --key-path --last --list --name --password --port --proxy --rename --reveal --set-option --sftp-allow-remote --sftp-start --sftp-stop --tags --user --version --webdav-start --webdav-stop -y --yes' -- "$cur"))
  else
    COMPREPLY=($(compgen -W "$profiles" -- "$cur"))
  fi
}
complete -F _qssh qssh
