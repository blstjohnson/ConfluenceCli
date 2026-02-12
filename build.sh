#!/bin/bash

# Script to build confcli binaries for multiple platforms using Docker

set -e  # Exit immediately if a command exits with a non-zero status

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
DOCKERFILE="Dockerfile.build"
IMAGE_NAME="confcli-builder"
CONTAINER_NAME="confcli-build-container"
DIST_DIR="dist"
BINARIES_DIR="$DIST_DIR/binaries"

# Print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Change to the project root directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed or not in PATH"
    exit 1
fi

# Check if we're in the right directory
if [[ ! -f "$DOCKERFILE" ]]; then
    print_error "Dockerfile '$DOCKERFILE' not found in current directory"
    exit 1
fi

# Create dist directory
mkdir -p "$BINARIES_DIR"

print_status "Building confcli binaries for multiple platforms..."

# Build the Docker image from the project root
print_status "Building Docker image..."
docker build -f "$DOCKERFILE" -t "$IMAGE_NAME" .

# Create a temporary container to extract binaries
print_status "Creating temporary container to extract binaries..."
docker create --name "$CONTAINER_NAME" "$IMAGE_NAME" > /dev/null

# Extract binaries from the container
print_status "Extracting binaries..."
docker cp "$CONTAINER_NAME:/dist/confcli-linux-amd64" "$BINARIES_DIR/"
docker cp "$CONTAINER_NAME:/dist/confcli-linux-arm64" "$BINARIES_DIR/"
docker cp "$CONTAINER_NAME:/dist/confcli-windows-amd64.exe" "$BINARIES_DIR/"
docker cp "$CONTAINER_NAME:/dist/confcli-windows-arm64.exe" "$BINARIES_DIR/"
docker cp "$CONTAINER_NAME:/dist/confcli-darwin-amd64" "$BINARIES_DIR/"
docker cp "$CONTAINER_NAME:/dist/confcli-darwin-arm64" "$BINARIES_DIR/"

# Clean up the temporary container
docker rm -f "$CONTAINER_NAME" > /dev/null

print_status "Binaries successfully extracted to $BINARIES_DIR/"

# Create release directory with organized structure
RELEASE_DIR="$DIST_DIR/releases"
mkdir -p "$RELEASE_DIR"

print_status "Creating release packages..."
cp "$BINARIES_DIR/confcli-linux-amd64" "$RELEASE_DIR/"
cp "$BINARIES_DIR/confcli-linux-arm64" "$RELEASE_DIR/"
cp "$BINARIES_DIR/confcli-windows-amd64.exe" "$RELEASE_DIR/"
cp "$BINARIES_DIR/confcli-windows-arm64.exe" "$RELEASE_DIR/"
cp "$BINARIES_DIR/confcli-darwin-amd64" "$RELEASE_DIR/"
cp "$BINARIES_DIR/confcli-darwin-arm64" "$RELEASE_DIR/"

print_status "Release binaries are available in $RELEASE_DIR/"

# List the created binaries
print_status "Created binaries:"
ls -la "$RELEASE_DIR/"

print_status "Build process completed successfully!"