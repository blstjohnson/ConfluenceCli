.PHONY: build test clean docker-build build-all

# Build for current platform
build:
	go build -o bin/confcli ./cmd/confcli

# Build for all platforms
build-all: build-linux-amd64 build-windows-amd64 build-darwin-amd64 build-darwin-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o dist/linux-amd64/confcli ./cmd/confcli

build-windows-amd64:
	GOOS=windows GOARCH=amd64 go build -o dist/windows-amd64/confcli.exe ./cmd/confcli

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -o dist/darwin-amd64/confcli ./cmd/confcli

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -o dist/darwin-arm64/confcli ./cmd/confcli

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -rf bin/ dist/

# Docker build
docker-build:
	docker build -t confcli-builder .
	docker run --rm -v ${PWD}/dist:/app/dist confcli-builder

# Install the binary locally
install: build
	cp bin/confcli $(GOPATH)/bin/confcli

# Run the application
run:
	go run cmd/confcli/main.go