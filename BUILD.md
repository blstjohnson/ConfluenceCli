# confcli Build Process

This document describes the build process for creating confcli binaries for multiple platforms using Docker.

## Overview

The build process creates binaries for the following platforms:
- Linux (AMD64 and ARM64)
- Windows (AMD64 and ARM64)
- macOS (AMD64 and ARM64)

## Prerequisites

- Docker installed and running (for cross-platform builds)
- Make (optional, for using the Makefile)
- Go 1.25+ (for native builds)

## Build Methods

### Method 1: Using Makefile (Cross-platform builds with Docker)

```bash
# Build all binaries using Docker
make build-docker

# Create release packages
make release

# Clean build artifacts
make clean
```

**Note:** Docker must be installed and running for cross-platform builds.

### Method 2: Native Go cross-compilation

If Docker is not available, you can build for different platforms using Go's built-in cross-compilation:

```bash
# Build using native Go cross-compilation
make build-native
```

This will build binaries for all platforms from your current OS.

### Method 3: Using the build script

```bash
# Run the build script directly (requires Docker)
./build.sh
```

### Method 4: Manual Docker build

```bash
# Build the Docker image
docker build -f Dockerfile.build -t confcli-builder .

# Create a container to extract binaries
docker create --name confcli-extract-container confcli-builder

# Extract binaries
docker cp confcli-extract-container:/dist/confcli-linux-amd64 ./dist/binaries/
docker cp confcli-extract-container:/dist/confcli-windows-amd64.exe ./dist/binaries/
docker cp confcli-extract-container:/dist/confcli-darwin-amd64 ./dist/binaries/
docker cp confcli-extract-container:/dist/confcli-linux-arm64 ./dist/binaries/
docker cp confcli-extract-container:/dist/confcli-windows-arm64.exe ./dist/binaries/
docker cp confcli-extract-container:/dist/confcli-darwin-arm64 ./dist/binaries/

# Clean up
docker rm -f confcli-extract-container
```

## Output

The build process creates the following directory structure:

```
dist/
├── binaries/          # Raw binaries
│   ├── confcli-linux-amd64
│   ├── confcli-linux-arm64
│   ├── confcli-windows-amd64.exe
│   ├── confcli-windows-arm64.exe
│   ├── confcli-darwin-amd64
│   └── confcli-darwin-arm64
└── releases/          # Release-ready binaries
    ├── confcli-linux-amd64
    ├── confcli-linux-arm64
    ├── confcli-windows-amd64.exe
    ├── confcli-windows-arm64.exe
    ├── confcli-darwin-amd64
    └── confcli-darwin-arm64
```

## Dockerfile.build

The Dockerfile.build uses a multi-stage build process:

1. **Builder Stage**: Builds binaries for all platforms using Go cross-compilation
2. **Final Stage**: Creates a minimal image with all binaries and packages them as ZIP files

## Cross-compilation Targets

The build process targets the following architectures:

- `GOOS=linux GOARCH=amd64` → confcli-linux-amd64
- `GOOS=linux GOARCH=arm64` → confcli-linux-arm64
- `GOOS=windows GOARCH=amd64` → confcli-windows-amd64.exe
- `GOOS=windows GOARCH=arm64` → confcli-windows-arm64.exe
- `GOOS=darwin GOARCH=amd64` → confcli-darwin-amd64
- `GOOS=darwin GOARCH=arm64` → confcli-darwin-arm64