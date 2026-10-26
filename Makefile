VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOVULNCHECK = golang.org/x/vuln/cmd/govulncheck@v1.9.0
BIN_DIR ?= $(HOME)/bin
COMPLETIONS_DIR ?= $(HOME)/.zsh/completions

.PHONY: build test run install uninstall

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/groundskeeper ./cmd/groundskeeper

test:
	go test ./...
	go run $(GOVULNCHECK) ./...

run: build
	./bin/groundskeeper scan --open

# Installs the binary, the gk alias and zsh completion for both names.
# The launchd schedule is added in build step 7.
install: build
	mkdir -p $(BIN_DIR) $(COMPLETIONS_DIR)
	install -m 0755 bin/groundskeeper $(BIN_DIR)/groundskeeper
	ln -sf groundskeeper $(BIN_DIR)/gk
	$(BIN_DIR)/groundskeeper completion zsh \
		| sed -e '1s/^#compdef groundskeeper$$/#compdef groundskeeper gk/' \
		      -e '2s/^compdef _groundskeeper groundskeeper$$/compdef _groundskeeper groundskeeper gk/' \
		> $(COMPLETIONS_DIR)/_groundskeeper

uninstall:
	rm -f $(BIN_DIR)/groundskeeper $(BIN_DIR)/gk $(COMPLETIONS_DIR)/_groundskeeper
