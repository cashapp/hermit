.NOTPARALLEL:

ROOT = $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BUILD_DIR = $(ROOT)/build
CHANNEL ?= canary
VERSION ?= $(shell git describe --tags --dirty  --always)
GOOS ?= $(shell ./bin/go env GOOS)
GOARCH ?= $(shell ./bin/go env GOARCH)
BIN = $(BUILD_DIR)/hermit-$(GOOS)-$(GOARCH)

.PHONY: all build lint test

all: lint test build

lint: ## run golangci-lint
	./bin/golangci-lint run

test: ## run tests
	./bin/go test -v ./...


build: build-prep build-$(GOOS)-$(GOARCH) ## builds binary and gzips it
	gzip -9 $(BIN)

build-prep:
	mkdir -p build

build-darwin-$(GOARCH):
	CGO_ENABLED=1 SDKROOT=$(shell xcrun --sdk macosx --show-sdk-path) \
		go build -trimpath -x -ldflags "-X main.version=$(VERSION) -X main.channel=$(CHANNEL)" -o $(BIN) ./cmd/hermit

build-linux-$(GOARCH):
	CGO_ENABLED=1 go build -trimpath -x -ldflags "-X main.version=$(VERSION) -X main.channel=$(CHANNEL)" -o $(BIN) ./cmd/hermit

help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_\/-]+:.*?## / {printf "\033[34m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST) | \
		sort | \
		grep -v '#'
