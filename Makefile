# Makefile for confcli build process

.PHONY: build build-docker build-all clean release

# Define binary names
BINARY_LINUX_AMD64 = confcli-linux-amd64
BINARY_LINUX_ARM64 = confcli-linux-arm64
BINARY_WINDOWS_AMD64 = confcli-windows-amd64.exe
BINARY_WINDOWS_ARM64 = confcli-windows-arm64.exe
BINARY_DARWIN_AMD64 = confcli-darwin-amd64
BINARY_DARWIN_ARM64 = confcli-darwin-arm64

# Define output directory
DIST_DIR = dist

# Build all binaries using Docker
build-docker:
	@echo "Building confcli binaries for multiple platforms using Docker..."
	@mkdir -p $(DIST_DIR)
	docker build -f Dockerfile.build -t confcli-builder .
	@echo "Extracting binaries from Docker container..."
	@mkdir -p $(DIST_DIR)/binaries
	@docker create --name confcli-extract-container confcli-builder
	@docker cp confcli-extract-container:/dist/$(BINARY_LINUX_AMD64) $(DIST_DIR)/binaries/
	@docker cp confcli-extract-container:/dist/$(BINARY_LINUX_ARM64) $(DIST_DIR)/binaries/
	@docker cp confcli-extract-container:/dist/$(BINARY_WINDOWS_AMD64) $(DIST_DIR)/binaries/
	@docker cp confcli-extract-container:/dist/$(BINARY_WINDOWS_ARM64) $(DIST_DIR)/binaries/
	@docker cp confcli-extract-container:/dist/$(BINARY_DARWIN_AMD64) $(DIST_DIR)/binaries/
	@docker cp confcli-extract-container:/dist/$(BINARY_DARWIN_ARM64) $(DIST_DIR)/binaries/
	@docker rm -f confcli-extract-container
	@echo "Binaries extracted to $(DIST_DIR)/binaries/"

# Build all binaries and create release packages
release: build-docker
	@echo "Creating release packages..."
	@mkdir -p $(DIST_DIR)/releases
	@cp $(DIST_DIR)/binaries/$(BINARY_LINUX_AMD64) $(DIST_DIR)/releases/
	@cp $(DIST_DIR)/binaries/$(BINARY_LINUX_ARM64) $(DIST_DIR)/releases/
	@cp $(DIST_DIR)/binaries/$(BINARY_WINDOWS_AMD64) $(DIST_DIR)/releases/
	@cp $(DIST_DIR)/binaries/$(BINARY_WINDOWS_ARM64) $(DIST_DIR)/releases/
	@cp $(DIST_DIR)/binaries/$(BINARY_DARWIN_AMD64) $(DIST_DIR)/releases/
	@cp $(DIST_DIR)/binaries/$(BINARY_DARWIN_ARM64) $(DIST_DIR)/releases/
	@echo "Release binaries are available in $(DIST_DIR)/releases/"

# Alternative method: Build directly with Go cross-compilation
build-native:
	@echo "Building confcli binaries natively..."
	@mkdir -p $(DIST_DIR)/native
	cd confcli-bin && GOOS=linux GOARCH=amd64 go build -o ../$(DIST_DIR)/native/$(BINARY_LINUX_AMD64) ./cmd/confcli
	cd confcli-bin && GOOS=linux GOARCH=arm64 go build -o ../$(DIST_DIR)/native/$(BINARY_LINUX_ARM64) ./cmd/confcli
	cd confcli-bin && GOOS=windows GOARCH=amd64 go build -o ../$(DIST_DIR)/native/$(BINARY_WINDOWS_AMD64) ./cmd/confcli
	cd confcli-bin && GOOS=windows GOARCH=arm64 go build -o ../$(DIST_DIR)/native/$(BINARY_WINDOWS_ARM64) ./cmd/confcli
	cd confcli-bin && GOOS=darwin GOARCH=amd64 go build -o ../$(DIST_DIR)/native/$(BINARY_DARWIN_AMD64) ./cmd/confcli
	cd confcli-bin && GOOS=darwin GOARCH=arm64 go build -o ../$(DIST_DIR)/native/$(BINARY_DARWIN_ARM64) ./cmd/confcli
	@echo "Native binaries built in $(DIST_DIR)/native/"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(DIST_DIR)
	@echo "Build artifacts cleaned."

# Show help
help:
	@echo "confcli build targets:"
	@echo "  build-docker - Build all binaries using Docker (recommended)"
	@echo "  build-native - Build all binaries using native Go cross-compilation"
	@echo "  release      - Build using Docker and create release packages"
	@echo "  clean        - Remove build artifacts"
	@echo "  help         - Show this help message"