.NOTPARALLEL:

ROOT = $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BUILD_DIR = $(ROOT)/build
CHANNEL ?= canary
VERSION ?= $(shell git describe --tags --dirty  --always)
GOOS ?= $(shell ./bin/go version | awk '{print $$NF}' | cut -d/ -f1)
GOARCH ?= $(shell ./bin/go version | awk '{print $$NF}' | cut -d/ -f2)
BIN = $(BUILD_DIR)/hermit-$(GOOS)-$(GOARCH)

.PHONY: all build lint test

all: lint test build

lint: ## run golangci-lint
	./bin/golangci-lint run

test: ## run tests
	./bin/go test -v ./...

build: ## builds binary and gzips it
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION) -X main.channel=$(CHANNEL)" -o $(BIN) $(ROOT)/cmd/hermit
	gzip -9 $(BIN)

help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_\/-]+:.*?## / {printf "\033[34m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST) | \
		sort | \
		grep -v '#'
