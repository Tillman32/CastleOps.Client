# CastleOps Client Makefile
# High-performance Go build configuration with cross-platform support

# Application metadata
APP_NAME := castleops-client
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go configuration
GO := go
GOFLAGS := -trimpath
LDFLAGS := -s -w \
	-X 'main.appVersion=$(VERSION)' \
	-X 'main.buildTime=$(BUILD_TIME)' \
	-X 'main.gitCommit=$(GIT_COMMIT)'

# CGO is required for sqlite3
CGO_ENABLED := 1

# Build directories
BUILD_DIR := build
DIST_DIR := dist
CMD_DIR := cmd/$(APP_NAME)

# Platform detection
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# Binary names
BINARY_UNIX := $(APP_NAME)
BINARY_DARWIN := $(APP_NAME)
BINARY_WINDOWS := $(APP_NAME).exe

# Colors for output
CYAN := \033[0;36m
GREEN := \033[0;32m
RED := \033[0;31m
YELLOW := \033[0;33m
NC := \033[0m # No Color

.PHONY: all build clean test bench vet fmt lint install run help \
	build-darwin build-windows build-linux \
	release check deps version

# Default target
all: clean deps build test

## help: Display this help message
help:
	@echo "$(CYAN)CastleOps Client Build System$(NC)"
	@echo ""
	@echo "$(GREEN)Common targets:$(NC)"
	@echo "  make build         - Build for current platform"
	@echo "  make build-darwin  - Build for macOS (amd64 and arm64)"
	@echo "  make build-windows - Build for Windows (amd64)"
	@echo "  make build-linux   - Build for Linux (amd64)"
	@echo "  make release       - Build release binaries for all platforms"
	@echo "  make test          - Run all tests"
	@echo "  make bench         - Run benchmarks"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make deps          - Download dependencies"
	@echo "  make run           - Build and run locally"
	@echo ""
	@echo "$(GREEN)Development targets:$(NC)"
	@echo "  make fmt           - Format code with gofmt"
	@echo "  make vet           - Run go vet"
	@echo "  make lint          - Run golangci-lint (if installed)"
	@echo "  make check         - Run fmt, vet, and tests"
	@echo ""
	@echo "$(YELLOW)Current platform:$(NC) $(GOOS)/$(GOARCH)"
	@echo "$(YELLOW)Version:$(NC) $(VERSION)"

## build: Build binary for current platform with optimization flags
build:
	@echo "$(CYAN)Building $(APP_NAME) for $(GOOS)/$(GOARCH)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP_NAME)$(if $(findstring windows,$(GOOS)),.exe,) \
		./$(CMD_DIR)
	@echo "$(GREEN)Build complete: $(BUILD_DIR)/$(APP_NAME)$(if $(findstring windows,$(GOOS)),.exe,)$(NC)"

## build-darwin: Build optimized binaries for macOS (amd64 and arm64)
build-darwin:
	@echo "$(CYAN)Building for macOS (darwin/amd64)...$(NC)"
	@mkdir -p $(DIST_DIR)/darwin-amd64
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-ldflags "$(LDFLAGS)" \
		-o $(DIST_DIR)/darwin-amd64/$(BINARY_DARWIN) \
		./$(CMD_DIR)
	@echo "$(GREEN)Built: $(DIST_DIR)/darwin-amd64/$(BINARY_DARWIN)$(NC)"

	@echo "$(CYAN)Building for macOS (darwin/arm64)...$(NC)"
	@mkdir -p $(DIST_DIR)/darwin-arm64
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) \
		-ldflags "$(LDFLAGS)" \
		-o $(DIST_DIR)/darwin-arm64/$(BINARY_DARWIN) \
		./$(CMD_DIR)
	@echo "$(GREEN)Built: $(DIST_DIR)/darwin-arm64/$(BINARY_DARWIN)$(NC)"

## build-windows: Build optimized binary for Windows (amd64)
build-windows:
	@echo "$(CYAN)Building for Windows (windows/amd64)...$(NC)"
	@mkdir -p $(DIST_DIR)/windows-amd64
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc $(GO) build $(GOFLAGS) \
		-ldflags "$(LDFLAGS)" \
		-o $(DIST_DIR)/windows-amd64/$(BINARY_WINDOWS) \
		./$(CMD_DIR)
	@echo "$(GREEN)Built: $(DIST_DIR)/windows-amd64/$(BINARY_WINDOWS)$(NC)"
	@echo "$(YELLOW)Note: Cross-compiling for Windows requires mingw-w64$(NC)"

## build-linux: Build optimized binary for Linux (amd64)
build-linux:
	@echo "$(CYAN)Building for Linux (linux/amd64)...$(NC)"
	@mkdir -p $(DIST_DIR)/linux-amd64
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) \
		-ldflags "$(LDFLAGS)" \
		-o $(DIST_DIR)/linux-amd64/$(BINARY_UNIX) \
		./$(CMD_DIR)
	@echo "$(GREEN)Built: $(DIST_DIR)/linux-amd64/$(BINARY_UNIX)$(NC)"

## release: Build release binaries for all platforms
release: clean
	@echo "$(CYAN)Building release for all platforms...$(NC)"
	@$(MAKE) build-darwin
	@$(MAKE) build-linux
	@echo "$(GREEN)Release builds complete in $(DIST_DIR)/$(NC)"
	@echo "$(YELLOW)Note: Windows build requires mingw-w64, run 'make build-windows' separately$(NC)"

## run: Build and run the application locally
run: build
	@echo "$(CYAN)Running $(APP_NAME)...$(NC)"
	@./$(BUILD_DIR)/$(APP_NAME)$(if $(findstring windows,$(GOOS)),.exe,)

## install: Install binary to GOPATH/bin
install:
	@echo "$(CYAN)Installing $(APP_NAME)...$(NC)"
	CGO_ENABLED=$(CGO_ENABLED) $(GO) install $(GOFLAGS) \
		-ldflags "$(LDFLAGS)" \
		./$(CMD_DIR)
	@echo "$(GREEN)Installed to $(shell go env GOPATH)/bin/$(APP_NAME)$(NC)"

## test: Run all tests with race detection
test:
	@echo "$(CYAN)Running tests...$(NC)"
	CGO_ENABLED=1 $(GO) test -race -v -cover ./...
	@echo "$(GREEN)Tests complete$(NC)"

## test-short: Run tests without race detection (faster)
test-short:
	@echo "$(CYAN)Running short tests...$(NC)"
	$(GO) test -short -v ./...
	@echo "$(GREEN)Tests complete$(NC)"

## bench: Run benchmarks with memory allocation stats
bench:
	@echo "$(CYAN)Running benchmarks...$(NC)"
	$(GO) test -bench=. -benchmem -benchtime=5s ./...
	@echo "$(GREEN)Benchmarks complete$(NC)"

## bench-cpu: Run benchmarks with CPU profiling
bench-cpu:
	@echo "$(CYAN)Running benchmarks with CPU profiling...$(NC)"
	@mkdir -p $(BUILD_DIR)/profiles
	$(GO) test -bench=. -benchmem -cpuprofile=$(BUILD_DIR)/profiles/cpu.prof ./...
	@echo "$(GREEN)CPU profile: $(BUILD_DIR)/profiles/cpu.prof$(NC)"
	@echo "$(YELLOW)View with: go tool pprof $(BUILD_DIR)/profiles/cpu.prof$(NC)"

## bench-mem: Run benchmarks with memory profiling
bench-mem:
	@echo "$(CYAN)Running benchmarks with memory profiling...$(NC)"
	@mkdir -p $(BUILD_DIR)/profiles
	$(GO) test -bench=. -benchmem -memprofile=$(BUILD_DIR)/profiles/mem.prof ./...
	@echo "$(GREEN)Memory profile: $(BUILD_DIR)/profiles/mem.prof$(NC)"
	@echo "$(YELLOW)View with: go tool pprof $(BUILD_DIR)/profiles/mem.prof$(NC)"

## vet: Run go vet to find potential issues
vet:
	@echo "$(CYAN)Running go vet...$(NC)"
	$(GO) vet ./...
	@echo "$(GREEN)Vet complete$(NC)"

## fmt: Format all Go files
fmt:
	@echo "$(CYAN)Formatting code...$(NC)"
	$(GO) fmt ./...
	@echo "$(GREEN)Formatting complete$(NC)"

## lint: Run golangci-lint (requires golangci-lint to be installed)
lint:
	@echo "$(CYAN)Running golangci-lint...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout 5m ./...; \
		echo "$(GREEN)Linting complete$(NC)"; \
	else \
		echo "$(YELLOW)golangci-lint not installed. Install with:$(NC)"; \
		echo "  brew install golangci-lint  # macOS"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## check: Run formatting, vetting, and tests
check: fmt vet test
	@echo "$(GREEN)All checks passed$(NC)"

## deps: Download and verify dependencies
deps:
	@echo "$(CYAN)Downloading dependencies...$(NC)"
	$(GO) mod download
	$(GO) mod verify
	@echo "$(GREEN)Dependencies ready$(NC)"

## tidy: Clean up dependencies
tidy:
	@echo "$(CYAN)Tidying dependencies...$(NC)"
	$(GO) mod tidy
	@echo "$(GREEN)Dependencies tidied$(NC)"

## clean: Remove build artifacts
clean:
	@echo "$(CYAN)Cleaning build artifacts...$(NC)"
	@rm -rf $(BUILD_DIR) $(DIST_DIR)
	@$(GO) clean -cache -testcache
	@echo "$(GREEN)Clean complete$(NC)"

## version: Display version information
version:
	@echo "$(CYAN)Version Information:$(NC)"
	@echo "  Version:    $(VERSION)"
	@echo "  Git Commit: $(GIT_COMMIT)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Platform:   $(GOOS)/$(GOARCH)"

# Development workflow shortcuts
.PHONY: dev watch
## dev: Development mode - format, vet, build, and run
dev: fmt vet build run

## watch: Watch for changes and rebuild (requires entr or similar)
watch:
	@echo "$(YELLOW)Watch mode requires 'entr' to be installed$(NC)"
	@echo "  brew install entr  # macOS"
	@echo "  apt-get install entr  # Ubuntu/Debian"
	@if command -v entr >/dev/null 2>&1; then \
		echo "$(CYAN)Watching for changes...$(NC)"; \
		find . -name '*.go' | entr -r make dev; \
	fi
