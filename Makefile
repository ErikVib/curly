.PHONY: test test-unit test-integration test-coverage test-race build clean help

# Default target
help:
	@echo "Available targets:"
	@echo "  make test              - Run all tests"
	@echo "  make test-unit         - Run unit tests only"
	@echo "  make test-integration  - Run integration tests"
	@echo "  make test-race         - Run tests with race detector"
	@echo "  make build             - Build the binary"
	@echo "  make clean             - Remove build artifacts"
	@echo "  make install           - Install the binary"

# Run all tests
test:
	go test -v ./...

# Run unit tests only (fast)
test-unit:
	go test -v -short ./...

# Run integration tests
test-integration:
	go test -v -tags=integration ./...

# Run tests with race detector
test-race:
	go test -v -race ./...

# Build the binary
build:
	go build -o curly main.go
	@echo "Binary built: ./curly"

# Install the binary
install:
	go install

# Clean build artifacts
clean:
	rm -r collection/
	@echo "Cleaned build artifacts"

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Tidy dependencies
tidy:
	go mod tidy

# Verify dependencies
verify:
	go mod verify
