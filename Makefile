BINARY    := paruz
MODULE    := github.com/vyogami/paruz
MAIN      := ./cmd/paruz
BUILD_DIR := .

GO       := go
GOFLAGS  := -trimpath
LDFLAGS  := -s -w
CGO      := 0

.DEFAULT_GOAL := help

.PHONY: help build run clean fmt lint vet tidy test install uninstall

## build: Compile the binary
build:
	CGO_ENABLED=$(CGO) $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY) $(MAIN)

## run: Build and run
run: build
	./$(BINARY)

## clean: Remove build artifacts
clean:
	rm -f $(BUILD_DIR)/$(BINARY)

## fmt: Format source code
fmt:
	$(GO) fmt ./...

## lint: Run staticcheck (install with: go install honnef.co/go/tools/cmd/staticcheck@latest)
lint:
	staticcheck ./...

## vet: Run go vet
vet:
	$(GO) vet ./...

## tidy: Tidy and verify module dependencies
tidy:
	$(GO) mod tidy
	$(GO) mod verify

## test: Run tests
test:
	$(GO) test -race ./...

## install: Install binary to GOPATH/bin
install:
	CGO_ENABLED=$(CGO) $(GO) install $(GOFLAGS) -ldflags '$(LDFLAGS)' $(MAIN)

## uninstall: Remove binary from GOPATH/bin
uninstall:
	rm -f $(shell $(GO) env GOPATH)/bin/$(BINARY)

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
