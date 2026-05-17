BINARY   = task
BUILD_DIR = ./cmd/task
INSTALL_DIR = $(HOME)/bin

.PHONY: build install clean test lint

build:
	go build -ldflags="-s -w" -o $(BINARY) $(BUILD_DIR)

install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

# Cross-compile release binaries
release:
	GOOS=darwin  GOARCH=arm64  go build -ldflags="-s -w" -o dist/task-darwin-arm64  $(BUILD_DIR)
	GOOS=darwin  GOARCH=amd64  go build -ldflags="-s -w" -o dist/task-darwin-amd64  $(BUILD_DIR)
	GOOS=linux   GOARCH=amd64  go build -ldflags="-s -w" -o dist/task-linux-amd64   $(BUILD_DIR)
	@echo "Binaries in dist/"

clean:
	rm -f $(BINARY)
	rm -rf dist/

test:
	go test ./...

lint:
	go vet ./...
