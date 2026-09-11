## FakeServer — Makefile

APP     := fakeserver
MAIN_DIR := ./cmd/fakeserver
GOEXE = $(shell go env GOEXE)
BINARY  := $(APP)$(GOEXE)

# Build metadata
BUILD_TIME := $(shell date +%Y-%m-%dT%H:%M:%S)
GIT_HASH  := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "unknown")
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev-$(GIT_HASH)")

LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitHash=$(GIT_HASH) \
	-X 'main.BuildTime=$(BUILD_TIME)'

# Install dir: prefer `go env GOBIN` (also covers `go env -w`), else GOPATH/bin.
# On Windows go env returns D:\... paths; the shell would eat the backslashes, so use forward slashes.
GOBIN_DIR := $(subst \,/,$(or $(shell go env GOBIN),$(shell go env GOPATH)/bin))
.PHONY: all build backend clean help install fmt-check vet test check

## all: build (default)
all: build

## build: build Go binary (current platform)
build:
	@echo "🐹 Building Go binary ($(VERSION) @ $(GIT_HASH))..."
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(MAIN_DIR)
	@if command -v upx >/dev/null 2>&1; then upx -6 --no-progress $(BINARY); else echo "⏭ upx not found; skipping compression"; fi
	@echo "✅ Binary: $(BINARY) ($$(du -sh $(BINARY) | cut -f1))"

## install: install Go binary to GOBIN (or GOPATH/bin)
install:
	go install -ldflags "$(LDFLAGS)" $(MAIN_DIR)
	@if command -v upx >/dev/null 2>&1; then upx -6 --no-progress "$(GOBIN_DIR)/$(BINARY)" || echo "⚠ upx failed; keeping the uncompressed binary"; else echo "⏭ upx not found; skipping compression"; fi
	@echo "✅ Installed to $(GOBIN_DIR)"

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)
vet:
	go vet ./...
test:
	go test ./...
check: fmt-check vet test

## run: build and run with current directory
run: build
	./$(BINARY)

# ─── Cross Compilation ────────────────────────────────────────────────────────

DIST_DIR := dist
DESCRIPTION := "Configurable HTTP mock/fake server"

## build-all: cross-compile for all platforms
build-all: clean-dist dump-info build-linux build-linux-arm64 build-darwin build-darwin-arm64 build-windows latest-yaml

## dump-info: dump build info
dump-info:
	@echo "Build Info:"
	@echo "  VERSION: $(VERSION)"
	@echo "  GIT_HASH: $(GIT_HASH)"
	@echo "  BUILD_TIME: $(BUILD_TIME)"

## latest-yaml: generate latest.yaml release metadata
latest-yaml:
	@mkdir -p $(DIST_DIR)
	@{ \
		echo "name: $(APP)"; \
		echo "version: $(VERSION)"; \
		echo "released_at: $(BUILD_TIME)"; \
		echo "description: $(DESCRIPTION)"; \
	} > $(DIST_DIR)/latest.yaml
	@echo "   → $(DIST_DIR)/latest.yaml"

## build-linux: compile for Linux amd64
build-linux:
	@echo "🐧 linux/amd64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-linux-amd64 $(MAIN_DIR)
	upx -6 --no-progress $(DIST_DIR)/$(APP)-linux-amd64
	chmod +x $(DIST_DIR)/$(APP)-linux-amd64
	@echo "   → $(DIST_DIR)/$(APP)-linux-amd64"

## build-linux-arm64: compile for Linux arm64
build-linux-arm64:
	@echo "🐧 linux/arm64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-linux-arm64 $(MAIN_DIR)
	upx -6 --no-progress $(DIST_DIR)/$(APP)-linux-arm64
	chmod +x $(DIST_DIR)/$(APP)-linux-arm64
	@echo "   → $(DIST_DIR)/$(APP)-linux-arm64"

## build-darwin: compile for macOS amd64
build-darwin:
	@echo "🍎 darwin/amd64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-darwin-amd64 $(MAIN_DIR)
	@echo "   → $(DIST_DIR)/$(APP)-darwin-amd64"

## build-darwin-arm64: compile for macOS Apple Silicon
build-darwin-arm64:
	@echo "🍎 darwin/arm64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-darwin-arm64 $(MAIN_DIR)
	# upx -6 --no-progress $(DIST_DIR)/$(APP)-darwin-arm64 # 压缩有问题在 macos 12+
	@echo "   → $(DIST_DIR)/$(APP)-darwin-arm64"

## build-windows: compile for Windows amd64
build-windows:
	@echo "🪟 windows/amd64..."
	@mkdir -p $(DIST_DIR)
	@GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)-windows-amd64.exe $(MAIN_DIR)
	upx -6 --no-progress $(DIST_DIR)/$(APP)-windows-amd64.exe
	@echo "   → $(DIST_DIR)/$(APP)-windows-amd64.exe"

.PHONY: release
release: build-all ## Create release archives for all platforms
	@echo "Creating release archives..."
	@mkdir -p release
	@cd $(DIST_DIR) && \
	tar -czf ../release/$(APP)-$(VERSION)-linux-amd64.zip $(APP)-linux-amd64; \
	tar -czf ../release/$(APP)-$(VERSION)-linux-arm64.zip $(APP)-linux-arm64; \
	tar -czf ../release/$(APP)-$(VERSION)-darwin-amd64.zip $(APP)-darwin-amd64; \
	tar -czf ../release/$(APP)-$(VERSION)-darwin-arm64.zip $(APP)-darwin-arm64; \
	zip ../release/$(APP)-$(VERSION)-windows-amd64.zip $(APP)-windows-amd64.exe; \
	# zip ../release/$(APP)-$(VERSION)-windows-arm64.zip $(APP)-windows-arm64.exe; \
	@echo "Release archives created in release/"

## clean: remove build artifacts
clean:
	@rm -f $(BINARY)
	@rm -rf $(DIST_DIR)
	@echo "🧹 Cleaned"

## clean-dist: remove old dist files
clean-dist:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@echo "🧹 Cleaned $(DIST_DIR)"

## help: show this help
help:
	@echo "Skillc Build System"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
