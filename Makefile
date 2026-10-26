VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOVULNCHECK = golang.org/x/vuln/cmd/govulncheck@v1.9.0
BIN_DIR ?= $(HOME)/bin
COMPLETIONS_DIR ?= $(HOME)/.zsh/completions
LABEL = com.lot86.groundskeeper
PLIST = $(HOME)/Library/LaunchAgents/$(LABEL).plist
LAUNCHD_LOG = $(HOME)/Library/Logs/groundskeeper.launchd.log
DOMAIN = gui/$(shell id -u)

.PHONY: build test run install uninstall

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/groundskeeper ./cmd/groundskeeper
	swift build -c release --package-path helpers/photos-helper
	install -m 0755 helpers/photos-helper/.build/release/photos-helper bin/photos-helper

test:
	go test ./...
	go run $(GOVULNCHECK) ./...

run: build
	./bin/groundskeeper scan --open

# Installs the binary, the gk alias, zsh completion for both names, and the
# LaunchAgent that runs the read-only scan on Mondays and Thursdays.
install: build
	mkdir -p $(BIN_DIR) $(COMPLETIONS_DIR) $(dir $(PLIST))
	install -m 0755 bin/groundskeeper $(BIN_DIR)/groundskeeper
	ln -sf groundskeeper $(BIN_DIR)/gk
	$(BIN_DIR)/groundskeeper completion zsh \
		| sed -e '1s/^#compdef groundskeeper$$/#compdef groundskeeper gk/' \
		      -e '2s/^compdef _groundskeeper groundskeeper$$/compdef _groundskeeper groundskeeper gk/' \
		> $(COMPLETIONS_DIR)/_groundskeeper
	sed 's|__HOME__|$(HOME)|g' launchd/$(LABEL).plist > $(PLIST)
	chmod 0644 $(PLIST)
	plutil -lint $(PLIST)
	# launchd would create its log readable by everyone; create it private first.
	mkdir -p $(dir $(LAUNCHD_LOG))
	touch $(LAUNCHD_LOG)
	chmod 0600 $(LAUNCHD_LOG)
	-launchctl bootout $(DOMAIN)/$(LABEL) 2>/dev/null
	launchctl bootstrap $(DOMAIN) $(PLIST)

uninstall:
	-launchctl bootout $(DOMAIN)/$(LABEL) 2>/dev/null
	rm -f $(PLIST) $(BIN_DIR)/groundskeeper $(BIN_DIR)/gk $(COMPLETIONS_DIR)/_groundskeeper
