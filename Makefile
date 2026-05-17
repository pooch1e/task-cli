BINARY    = task
BUILD_DIR = ./cmd/task
INSTALL_DIR = $(HOME)/.local/bin

# Embed the git tag (falls back to commit hash when no tags exist).
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   = -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build install clean test lint release checksums snapshot

## build: compile for the current platform
build:
	go build $(LDFLAGS) -o $(BINARY) $(BUILD_DIR)

## install: build and copy to INSTALL_DIR (defaults to ~/.local/bin)
install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) $(VERSION) → $(INSTALL_DIR)/$(BINARY)"

## snapshot: quick build without version injection (fast dev loop)
snapshot:
	go build -o $(BINARY) $(BUILD_DIR)

## release: cross-compile for all supported targets with version embedded
release:
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o dist/task-darwin-arm64  $(BUILD_DIR)
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o dist/task-darwin-amd64  $(BUILD_DIR)
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o dist/task-linux-amd64   $(BUILD_DIR)
	@echo "Built $(VERSION) → dist/"

## checksums: generate sha256 checksums for all dist/ binaries
checksums: release
	@cd dist && sha256sum task-* > checksums.txt
	@echo "Checksums written to dist/checksums.txt"

## clean: remove build artefacts
clean:
	rm -f $(BINARY)
	rm -rf dist/

## test: run the full test suite
test:
	go test ./...

## lint: run go vet
lint:
	go vet ./...

## help: list available targets with descriptions
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
