BINARY    = task
BUILD_DIR = ./cmd/task
INSTALL_DIR = $(HOME)/.local/bin

# Embed the git tag (falls back to commit hash when no tags exist).
# --dirty is intentionally excluded — it makes checksums non-deterministic.
VERSION  ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")

# Quote the version string to survive spaces/special chars in shell expansion.
LDFLAGS   = -ldflags "-s -w -X 'main.version=$(VERSION)'"

.PHONY: build install snapshot release checksums clean test lint help

## build: compile for the current platform with version embedded
build:
	go build $(LDFLAGS) -o $(BINARY) $(BUILD_DIR)

## install: build and copy to INSTALL_DIR (default: ~/.local/bin)
install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@test -x $(INSTALL_DIR)/$(BINARY) || chmod +x $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) $(VERSION) → $(INSTALL_DIR)/$(BINARY)"

## snapshot: fast dev build without version injection
snapshot:
	go build -o $(BINARY) $(BUILD_DIR)

## release: cross-compile all supported targets with version embedded
release:
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o dist/task-darwin-arm64  $(BUILD_DIR)
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o dist/task-darwin-amd64  $(BUILD_DIR)
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o dist/task-linux-amd64   $(BUILD_DIR)
	@echo "Built $(VERSION) → dist/"

## checksums: generate sha256 checksums for dist/ binaries (after release)
# Works on both Linux (sha256sum) and macOS (shasum -a 256).
checksums:
	@test -f dist/task-darwin-arm64 && \
	 test -f dist/task-darwin-amd64 && \
	 test -f dist/task-linux-amd64  || \
	 (echo "error: run 'make release' first" >&2 && exit 1)
	@cd dist && \
	if command -v sha256sum >/dev/null 2>&1; then \
	  sha256sum task-* > checksums.txt; \
	else \
	  shasum -a 256 task-* > checksums.txt; \
	fi
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

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
