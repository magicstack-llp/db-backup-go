# Makefile for db-backup-go

# Build configuration
BINARY_NAME=db-backup
BUILD_DIR=build
VERSION?=0.1.1
BUILD_TIME=$(shell date +%Y%m%d-%H%M%S)
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# LDFLAGS for version information
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

.PHONY: all build clean test deps help build-dir

# Default target
all: build

# Create build directory
build-dir:
	@mkdir -p $(BUILD_DIR)

# Build for current platform
build: build-dir
	@echo "Building $(BINARY_NAME) for current platform..."
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for Linux AMD64
build-linux: build-dir
	@echo "Building $(BINARY_NAME) for Linux AMD64..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

# Build for Linux ARM64
build-linux-arm64: build-dir
	@echo "Building $(BINARY_NAME) for Linux ARM64..."
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

# Build for macOS AMD64
build-darwin-amd64: build-dir
	@echo "Building $(BINARY_NAME) for macOS AMD64..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64"

# Build for macOS ARM64 (Apple Silicon)
build-darwin-arm64: build-dir
	@echo "Building $(BINARY_NAME) for macOS ARM64..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64"

# Build for Windows AMD64
build-windows: build-dir
	@echo "Building $(BINARY_NAME) for Windows AMD64..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe"

# Build for all platforms
build-all: build-linux build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows
	@echo "All platform builds complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Install dependencies
install-deps: deps

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) . && $(BUILD_DIR)/$(BINARY_NAME) --help

# Show help
help:
	@echo "Available targets:"
	@echo "  build              - Build for current platform (outputs to $(BUILD_DIR)/)"
	@echo "  build-linux        - Build for Linux AMD64"
	@echo "  build-linux-arm64  - Build for Linux ARM64"
	@echo "  build-darwin-amd64 - Build for macOS AMD64"
	@echo "  build-darwin-arm64 - Build for macOS ARM64 (Apple Silicon)"
	@echo "  build-windows      - Build for Windows AMD64"
	@echo "  build-all          - Build for all platforms"
	@echo "  clean              - Remove build artifacts"
	@echo "  test               - Run tests"
	@echo "  deps               - Download and tidy dependencies"
	@echo "  run                - Build and run the application"
	@echo "  help               - Show this help message"

