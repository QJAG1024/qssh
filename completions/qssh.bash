# bash completion for qssh
_qssh() {
  local cur prev words cword
  _init_completion || return
  local profiles=
  case $prev in
    --add) return ;;
    --edit|--delete)
      COMPREPLY=()
      return ;;
    --list)
      COMPREPLY=()
      return ;;
  esac
  if [[ $cur == -* ]]; then
    COMPREPLY=()
  else
    COMPREPLY=()
  fi
}
complete -F _qssh qssh
