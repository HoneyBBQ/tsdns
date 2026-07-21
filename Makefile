.PHONY: all build test lint fmt clean help

# Default target
all: help

## build: Build the binary
build:
	go build -v -o bin/tsdns ./cmd/tsdns

## test: Run all unit tests
test:
	go test -v -race -cover ./...
	go test -v -race -cover ./core/...

## lint: Run golangci-lint static analysis
lint:
	golangci-lint run
	cd core && golangci-lint run --config ../.golangci.yml

## fmt: Format code and optimize imports
fmt:
	go fmt ./...
	go -C core fmt ./...
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
