BINARY := qssh
PREFIX ?= /usr/local
GOFLAGS := -ldflags="-s -w" -trimpath

.PHONY: all build install test clean

all: build

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BINARY) .

install: build completions
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/
	# ZSH completions
	install -d $(DESTDIR)$(PREFIX)/share/zsh/site-functions/
	install -m 644 completions/_qssh $(DESTDIR)$(PREFIX)/share/zsh/site-functions/
	# Bash completions
	install -d $(DESTDIR)/usr/share/bash-completion/completions/
	install -m 644 completions/qssh.bash $(DESTDIR)/usr/share/bash-completion/completions/qssh
	@echo "qssh installed."

completions: completions/_qssh completions/qssh.bash

completions/_qssh:
	@mkdir -p completions
	@printf '%s\n' \
	  '#compdef qssh' \
	  '_qssh() {' \
	  '  local -a profiles' \
	  '  profiles=(${(f)"$(_call_program profiles qssh --list 2>/dev/null | tail -n +3 | awk "{print \$1}")"})' \
	  '  _arguments \' \
	  '    "--add[Create a new profile]:name:" \' \
	  '    "--edit[Edit an existing profile]:name:(${profiles[@]})" \' \
	  '    "--delete[Delete a profile]:name:(${profiles[@]})" \' \
	  '    "--list[List profiles]:filter:(${profiles[@]})" \' \
	  '    "--version[Print version]" \' \
	  '    "*: :->profile"' \
	  '  case $$state in' \
	  '    profile)' \
	  '      _describe profiles profiles' \
	  '      ;;' \
	  '  esac' \
	  '}' \
	  '_qssh "$$@"' > completions/_qssh
	@chmod 644 completions/_qssh

completions/qssh.bash:
	@mkdir -p completions
	@printf '%s\n' \
	  '# bash completion for qssh' \
	  '_qssh() {' \
	  '  local cur prev words cword' \
	  '  _init_completion || return' \
	  '  local profiles=$(qssh --list 2>/dev/null | tail -n +3 | awk "{print \$1}" | tr "\\n" " ")' \
	  '  case $$prev in' \
	  '    --add) return ;;' \
	  '    --edit|--delete)' \
	  '      COMPREPLY=($(compgen -W "$$profiles" -- "$$cur"))' \
	  '      return ;;' \
	  '    --list)' \
	  '      COMPREPLY=($(compgen -W "$$profiles" -- "$$cur"))' \
	  '      return ;;' \
	  '  esac' \
	  '  if [[ $$cur == -* ]]; then' \
	  '    COMPREPLY=($(compgen -W "--add --edit --delete --list --version" -- "$$cur"))' \
	  '  else' \
	  '    COMPREPLY=($(compgen -W "$$profiles" -- "$$cur"))' \
	  '  fi' \
	  '}' \
	  'complete -F _qssh qssh' > completions/qssh.bash
	@chmod 644 completions/qssh.bash

test:
	go test -v -race -count=1 ./...

clean:
	rm -f $(BINARY)
	rm -rf completions/
	go clean