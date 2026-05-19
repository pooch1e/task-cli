BINARY    = task
BUILD_DIR = ./cmd/task
INSTALL_DIR = $(HOME)/.local/bin
MAN_DIR   = /usr/local/share/man/man1
MAN_PAGE  = man/task-cli.1

# Embed the git tag (falls back to commit hash when no tags exist).
# --dirty is intentionally excluded — it makes checksums non-deterministic.
VERSION  ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")

# Quote the version string to survive spaces/special chars in shell expansion.
LDFLAGS   = -ldflags "-s -w -X 'main.version=$(VERSION)'"

.PHONY: build install man snapshot release checksums clean test lint help

## build: compile for the current platform with version embedded
build:
	go build $(LDFLAGS) -o $(BINARY) $(BUILD_DIR)

## man: generate man/task-cli.1 from cobra command metadata
man:
	@mkdir -p man
	go run ./cmd/gendocs/ -o man/
	@# cobra/doc names the file after the command Use field ("task.1"); rename it.
	@mv -f man/task.1 man/task-cli.1 2>/dev/null || true
	@echo "Man page written to man/task-cli.1"

## install: build, install binary, and install man page to MAN_DIR
install: build man
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@test -x $(INSTALL_DIR)/$(BINARY) || chmod +x $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) $(VERSION) → $(INSTALL_DIR)/$(BINARY)"
	@if [ -d "$(MAN_DIR)" ] || mkdir -p "$(MAN_DIR)" 2>/dev/null; then \
		cp $(MAN_PAGE) $(MAN_DIR)/task-cli.1; \
		echo "Man page installed → $(MAN_DIR)/task-cli.1"; \
	else \
		echo "Note: could not install man page (no write access to $(MAN_DIR))"; \
		echo "      Install manually: sudo cp $(MAN_PAGE) $(MAN_DIR)/task-cli.1"; \
	fi

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
	rm -rf dist/ man/

## test: run the full test suite
test:
	go test ./...

## lint: run go vet
lint:
	go vet ./...

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
