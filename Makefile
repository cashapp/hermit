.NOTPARALLEL:

ROOT = $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BUILD_DIR = $(ROOT)/build
CHANNEL ?= canary
VERSION ?= $(shell git describe --tags --dirty  --always)
GOOS ?= $(shell go version | awk '{print $NF}' | cut -d/ -f1)
GOARCH ?= $(shell go version | awk '{print $NF}' | cut -d/ -f2)
BIN = $(BUILD_DIR)/hermit-$(GOOS)-$(GOARCH)

.PHONY: all build lint test

all: lint test build

lint:
	./bin/golangci-lint run

test:
	./bin/go test -v ./...

build:
	mkdir -p build
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION) -X main.channel=$(CHANNEL)" -o $(BIN) ./cmd/hermit
	gzip -9 $(BIN)
