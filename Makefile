.PHONY: all build test lint fmt clean help

# Default target
all: help

## build: Build the binary
build:
	go build -v -o bin/tsdns ./cmd/tsdns

## test: Run all unit tests
test:
	go test -v -race -cover ./...

## lint: Run golangci-lint static analysis
lint:
	golangci-lint run

## fmt: Format code and optimize imports
fmt:
	go fmt ./...
	goimports -w .

## clean: Clean build artifacts
clean:
	rm -rf bin/
	rm -rf dist/

## help: Show help information
help: Makefile
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
