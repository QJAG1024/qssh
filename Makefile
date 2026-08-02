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

completions: completions/_qssh completions/qssh.bash completions/qssh.fish

completions/_qssh completions/qssh.bash completions/qssh.fish: cmd/gen_completions.go main.go
	@mkdir -p completions
	@go run cmd/gen_completions.go
	@chmod 644 completions/_qssh completions/qssh.bash completions/qssh.fish


test:
	go test -v -race -count=1 ./...

clean:
	rm -f $(BINARY)
	rm -rf completions/
	go clean