VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOVULNCHECK = golang.org/x/vuln/cmd/govulncheck@v1.9.0

.PHONY: build test run

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/groundskeeper ./cmd/groundskeeper

test:
	go test ./...
	go run $(GOVULNCHECK) ./...

run: build
	./bin/groundskeeper scan --open
