# Diffusion Makefile
# Build configuration for cross-platform compilation

# Binary name
BINARY_NAME=diffusion

# Version - extract semver from git tags (x.x.x format only)
# Strips git commit info and dirty flag, keeps only version number
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null | sed -E 's/^v?([0-9]+\.[0-9]+\.[0-9]+).*/\1/' || echo "0.0.0")

# Build flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"

# Output directory
BIN_DIR=./bin

# Detect current OS and ARCH
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Detect if we're on Windows to disable colors
# Check for common Windows indicators
IS_WINDOWS := $(if $(findstring Windows,$(OS)),1,0)
ifeq ($(IS_WINDOWS),0)
    IS_WINDOWS := $(if $(findstring windows,$(GOOS)),1,0)
endif

# Colors for Unix only
ifeq ($(IS_WINDOWS),1)
    # Windows - no colors
    COLOR_RESET=
    COLOR_BOLD=
    COLOR_GREEN=
    COLOR_YELLOW=
    COLOR_BLUE=
else
    # Unix - with colors
    COLOR_RESET=\033[0m
    COLOR_BOLD=\033[1m
    COLOR_GREEN=\033[32m
    COLOR_YELLOW=\033[33m
    COLOR_BLUE=\033[34m
endif

.PHONY: all build clean test dist help linux darwin windows \
        linux-amd64 linux-arm64 linux-arm \
        darwin-amd64 darwin-arm64 \
        windows-amd64 windows-arm64 windows-arm \
        version

# Default target
all: build

# Show version information
version:
	@echo "$(COLOR_BOLD)Diffusion Build Information$(COLOR_RESET)"
	@echo "Version:     $(COLOR_GREEN)$(VERSION)$(COLOR_RESET)"
	@echo "Go Version:  $(COLOR_BLUE)$(shell go version | cut -d' ' -f3)$(COLOR_RESET)"
	@echo "Build OS:    $(COLOR_BLUE)$(GOOS)$(COLOR_RESET)"
	@echo "Build Arch:  $(COLOR_BLUE)$(GOARCH)$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Build flags:$(COLOR_RESET)"
	@echo "LDFLAGS:     $(LDFLAGS)"

# Help target
help:
	@echo "$(COLOR_BOLD)Diffusion Build System$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Available targets:$(COLOR_RESET)"
	@echo "  $(COLOR_GREEN)build$(COLOR_RESET)          - Build for current platform"
	@echo "  $(COLOR_GREEN)dist$(COLOR_RESET)           - Build for all platforms (Linux, macOS, Windows)"
	@echo "  $(COLOR_GREEN)linux$(COLOR_RESET)          - Build for all Linux architectures"
	@echo "  $(COLOR_GREEN)darwin$(COLOR_RESET)         - Build for all macOS architectures"
	@echo "  $(COLOR_GREEN)windows$(COLOR_RESET)        - Build for all Windows architectures"
	@echo "  $(COLOR_GREEN)test$(COLOR_RESET)           - Run unit tests"
	@echo "  $(COLOR_GREEN)clean$(COLOR_RESET)          - Remove build artifacts"
	@echo "  $(COLOR_GREEN)version$(COLOR_RESET)        - Show version and build information"
	@echo ""
	@echo "$(COLOR_BOLD)Platform-specific targets:$(COLOR_RESET)"
	@echo "  $(COLOR_BLUE)linux-amd64$(COLOR_RESET)    - Build for Linux AMD64"
	@echo "  $(COLOR_BLUE)linux-arm64$(COLOR_RESET)    - Build for Linux ARM64"
	@echo "  $(COLOR_BLUE)linux-arm$(COLOR_RESET)      - Build for Linux ARM"
	@echo "  $(COLOR_BLUE)darwin-amd64$(COLOR_RESET)   - Build for macOS AMD64 (Intel)"
	@echo "  $(COLOR_BLUE)darwin-arm64$(COLOR_RESET)   - Build for macOS ARM64 (Apple Silicon)"
	@echo "  $(COLOR_BLUE)windows-amd64$(COLOR_RESET)  - Build for Windows AMD64"
	@echo "  $(COLOR_BLUE)windows-arm64$(COLOR_RESET)  - Build for Windows ARM64"
	@echo "  $(COLOR_BLUE)windows-arm$(COLOR_RESET)    - Build for Windows ARM"
	@echo ""
	@echo "$(COLOR_BOLD)Examples:$(COLOR_RESET)"
	@echo "  make build                    # Build for current platform"
	@echo "  make dist                     # Build for all platforms"
	@echo "  make linux                    # Build for all Linux architectures"
	@echo "  make VERSION=1.0.0 dist       # Build with specific version"
	@echo "  make version                  # Show version information"

# Build for current platform
build:
	@echo "$(COLOR_GREEN)Building $(BINARY_NAME) for $(GOOS)/$(GOARCH)...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)$(shell go env GOEXE) ./cmd/diffusion
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)$(shell go env GOEXE)$(COLOR_RESET)"

# Build for all platforms
dist: clean linux darwin windows
	@echo ""
	@echo "$(COLOR_BOLD)$(COLOR_GREEN)✓ Distribution build complete!$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Built binaries:$(COLOR_RESET)"
	@ls -lh $(BIN_DIR)/ 2>/dev/null || dir $(BIN_DIR)

# Linux builds
linux: linux-amd64 linux-arm64 linux-arm

linux-amd64:
	@echo "$(COLOR_YELLOW)Building for Linux AMD64...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/diffusion
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-linux-amd64$(COLOR_RESET)"

linux-arm64:
	@echo "$(COLOR_YELLOW)Building for Linux ARM64...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/diffusion
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-linux-arm64$(COLOR_RESET)"

linux-arm:
	@echo "$(COLOR_YELLOW)Building for Linux ARM...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm ./cmd/diffusion
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-linux-arm$(COLOR_RESET)"

# macOS builds
darwin: darwin-amd64 darwin-arm64

darwin-amd64:
	@echo "$(COLOR_YELLOW)Building for macOS AMD64 (Intel)...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64 .
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64$(COLOR_RESET)"

darwin-arm64:
	@echo "$(COLOR_YELLOW)Building for macOS ARM64 (Apple Silicon)...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64$(COLOR_RESET)"

# Windows builds
windows: windows-amd64 windows-arm64 windows-arm

windows-amd64:
	@echo "$(COLOR_YELLOW)Building for Windows AMD64...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe$(COLOR_RESET)"

windows-arm64:
	@echo "$(COLOR_YELLOW)Building for Windows ARM64...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-windows-arm64.exe .
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-windows-arm64.exe$(COLOR_RESET)"

windows-arm:
	@echo "$(COLOR_YELLOW)Building for Windows ARM...$(COLOR_RESET)"
	@mkdir -p $(BIN_DIR)
	@GOOS=windows GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-windows-arm.exe .
	@echo "$(COLOR_GREEN)✓ Built: $(BIN_DIR)/$(BINARY_NAME)-windows-arm.exe$(COLOR_RESET)"

# Run tests
test:
	@echo "$(COLOR_YELLOW)Running tests...$(COLOR_RESET)"
	@go test -v ./...
	@echo "$(COLOR_GREEN)✓ Tests passed$(COLOR_RESET)"

# Clean build artifacts
clean:
	@echo "$(COLOR_YELLOW)Cleaning build artifacts...$(COLOR_RESET)"
	@rm -rf $(BIN_DIR) 2>/dev/null || rmdir /s /q $(BIN_DIR) 2>nul || true
	@echo "$(COLOR_GREEN)✓ Clean complete$(COLOR_RESET)"

# Legacy targets for backward compatibility
linux_build: linux-amd64
	@echo "$(COLOR_YELLOW)Note: 'linux_build' is deprecated, use 'make linux-amd64' or 'make linux'$(COLOR_RESET)"

win_build: windows-amd64
	@echo "$(COLOR_YELLOW)Note: 'win_build' is deprecated, use 'make windows-amd64' or 'make windows'$(COLOR_RESET)"
