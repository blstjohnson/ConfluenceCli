# Makefile for confcli build process

.PHONY: build build-docker build-all clean release

# Define output directory
DIST_DIR = dist

# Build all binaries using Docker
build-docker:
	@echo "Building confcli binaries for multiple platforms using Docker..."
	@mkdir -p $(DIST_DIR)
	docker build -f Dockerfile.build -t confcli-builder .
	@echo "Extracting binaries from Docker container..."
	@mkdir -p $(DIST_DIR)/binaries/linux/amd64
	@mkdir -p $(DIST_DIR)/binaries/linux/arm64
	@mkdir -p $(DIST_DIR)/binaries/windows/amd64
	@mkdir -p $(DIST_DIR)/binaries/windows/arm64
	@mkdir -p $(DIST_DIR)/binaries/darwin/amd64
	@mkdir -p $(DIST_DIR)/binaries/darwin/arm64
	@docker create --name confcli-extract-container confcli-builder
	@docker cp confcli-extract-container:/dist/linux/amd64/confcli $(DIST_DIR)/binaries/linux/amd64/
	@docker cp confcli-extract-container:/dist/linux/arm64/confcli $(DIST_DIR)/binaries/linux/arm64/
	@docker cp confcli-extract-container:/dist/windows/amd64/confcli.exe $(DIST_DIR)/binaries/windows/amd64/
	@docker cp confcli-extract-container:/dist/windows/arm64/confcli.exe $(DIST_DIR)/binaries/windows/arm64/
	@docker cp confcli-extract-container:/dist/darwin/amd64/confcli $(DIST_DIR)/binaries/darwin/amd64/
	@docker cp confcli-extract-container:/dist/darwin/arm64/confcli $(DIST_DIR)/binaries/darwin/arm64/
	@docker rm -f confcli-extract-container
	@echo "Binaries extracted to $(DIST_DIR)/binaries/"

# Build all binaries and create release packages
release: build-docker
	@echo "Creating release packages..."
	@mkdir -p $(DIST_DIR)/releases/linux/amd64
	@mkdir -p $(DIST_DIR)/releases/linux/arm64
	@mkdir -p $(DIST_DIR)/releases/windows/amd64
	@mkdir -p $(DIST_DIR)/releases/windows/arm64
	@mkdir -p $(DIST_DIR)/releases/darwin/amd64
	@mkdir -p $(DIST_DIR)/releases/darwin/arm64
	@cp $(DIST_DIR)/binaries/linux/amd64/confcli $(DIST_DIR)/releases/linux/amd64/
	@cp $(DIST_DIR)/binaries/linux/arm64/confcli $(DIST_DIR)/releases/linux/arm64/
	@cp $(DIST_DIR)/binaries/windows/amd64/confcli.exe $(DIST_DIR)/releases/windows/amd64/
	@cp $(DIST_DIR)/binaries/windows/arm64/confcli.exe $(DIST_DIR)/releases/windows/arm64/
	@cp $(DIST_DIR)/binaries/darwin/amd64/confcli $(DIST_DIR)/releases/darwin/amd64/
	@cp $(DIST_DIR)/binaries/darwin/arm64/confcli $(DIST_DIR)/releases/darwin/arm64/
	@echo "Release binaries are available in $(DIST_DIR)/releases/"

# Alternative method: Build directly with Go cross-compilation
build-native:
	@echo "Building confcli binaries natively..."
	@mkdir -p $(DIST_DIR)/native/linux/amd64
	@mkdir -p $(DIST_DIR)/native/linux/arm64
	@mkdir -p $(DIST_DIR)/native/windows/amd64
	@mkdir -p $(DIST_DIR)/native/windows/arm64
	@mkdir -p $(DIST_DIR)/native/darwin/amd64
	@mkdir -p $(DIST_DIR)/native/darwin/arm64
	cd confcli-bin && GOOS=linux GOARCH=amd64 go build -o ../$(DIST_DIR)/native/linux/amd64/confcli ./cmd/confcli
	cd confcli-bin && GOOS=linux GOARCH=arm64 go build -o ../$(DIST_DIR)/native/linux/arm64/confcli ./cmd/confcli
	cd confcli-bin && GOOS=windows GOARCH=amd64 go build -o ../$(DIST_DIR)/native/windows/amd64/confcli.exe ./cmd/confcli
	cd confcli-bin && GOOS=windows GOARCH=arm64 go build -o ../$(DIST_DIR)/native/windows/arm64/confcli.exe ./cmd/confcli
	cd confcli-bin && GOOS=darwin GOARCH=amd64 go build -o ../$(DIST_DIR)/native/darwin/amd64/confcli ./cmd/confcli
	cd confcli-bin && GOOS=darwin GOARCH=arm64 go build -o ../$(DIST_DIR)/native/darwin/arm64/confcli ./cmd/confcli
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