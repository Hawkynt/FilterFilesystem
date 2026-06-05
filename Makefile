.PHONY: build test lint clean install deps fmt vet check all

# Build variables
BINARY_NAME=filterfs
BUILD_DIR=bin
MAIN_PACKAGE=./cmd/filterfs
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# Go build flags
GOFLAGS=-mod=readonly
GOTAGS=

# Default target
all: check build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

# Install dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Lint the code
lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Vet code
vet:
	@echo "Vetting code..."
	go vet ./...

# Run all checks
check: fmt vet lint test

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	go install $(GOFLAGS) $(LDFLAGS) $(MAIN_PACKAGE)

# Development mode - build and run with sample config
dev: build
	@echo "Running in development mode..."
	@mkdir -p tmp/source tmp/mount
	@echo "Sample content" > tmp/source/sample.txt
	@echo "Log content" > tmp/source/app.log
	@echo "Temp file" > tmp/source/temp.tmp
	./$(BUILD_DIR)/$(BINARY_NAME) mount -s tmp/source -m tmp/mount -b "**/*.log" -b "**/*.tmp" --log-level debug

# Example configuration content (define keeps the body verbatim, so it can be
# emitted from a recipe without tripping Make's tab-based recipe parsing)
define EXAMPLE_CONFIG
# FilterFS Configuration Example
# This file demonstrates all available configuration options

# Required: Path to the source directory to be filtered
source_path: /path/to/source/directory

# Required: Path where the filtered filesystem will be mounted
mount_path: /path/to/mount/point

# Optional: Mount the filesystem in read-only mode (default: false)
read_only: false

# Optional: List of glob patterns to blacklist (hide) files and directories
blacklist:
  # Hide all log files at any level
  - "**/*.log"
  
  # Hide all temporary files
  - "**/*.tmp"
  - "**/*.temp"
  
  # Hide version control directories
  - "**/.git"
  - "**/.svn"
  - "**/.hg"
  
  # Hide build/dependency directories
  - "**/node_modules"
  - "**/target"        # Rust/Java builds
  - "**/build"
  - "**/dist"
  
  # Hide cache files and directories
  - "**/*.cache"
  - "**/__pycache__"   # Python cache
  - "**/.pytest_cache" # Pytest cache
  
  # Hide OS-specific files
  - "**/Thumbs.db"     # Windows thumbnails
  - "**/.DS_Store"     # macOS metadata
  - "**/desktop.ini"   # Windows folder settings
  
  # Hide backup files
  - "**/*.bak"
  - "**/*.backup"
  - "**/*~"            # Emacs backups
  
  # Hide IDE/editor files
  - "**/.vscode"
  - "**/.idea"
  - "**/*.swp"         # Vim swap files
  - "**/*.swo"
  
  # Custom patterns (examples)
  - "*/secrets"        # Hide 'secrets' directories only in first sublevel
  - "/**/*.private"    # Hide .private files from root down

# Optional: Allow deletion of directories that contain hidden files (default: false)
# When false, attempts to delete directories with blacklisted content will fail
allow_delete_with_hidden: false

# Optional: Allow renaming of directories that contain hidden files (default: false)  
# When false, attempts to rename directories with blacklisted content will fail
allow_rename_with_hidden: false
endef
export EXAMPLE_CONFIG

# Create example config
example-config:
	@echo "Creating example configuration..."
	@printf '%s\n' "$$EXAMPLE_CONFIG" > filterfs.example.yaml
	@echo "Example configuration created: filterfs.example.yaml"

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t filterfs:$(VERSION) .

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  bench         - Run benchmarks"
	@echo "  lint          - Run linters"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  check         - Run all checks (fmt, vet, lint, test)"
	@echo "  clean         - Clean build artifacts"
	@echo "  install       - Install the binary"
	@echo "  dev           - Run in development mode"
	@echo "  deps          - Download dependencies"
	@echo "  example-config - Create example configuration file"
	@echo "  build-all     - Build for multiple platforms"
	@echo "  docker-build  - Build Docker image"
	@echo "  all           - Run checks and build (default)"
	@echo "  help          - Show this help"