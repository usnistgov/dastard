GOCMD=go
GOFMT=$(GOCMD) fmt
GOGET=$(GOCMD) get
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=dastard

.PHONY: all build install test clean run deps static

LDFLAGS=-ldflags "-X main.buildDate=$(shell date -u '+%Y-%m-%d.%H:%M:%S.%Z') -X main.githash=$(shell git rev-parse --short HEAD)"
build: $(BINARY_NAME)
all: test build install

$(BINARY_NAME): Makefile *.go cmd/dastard/dastard.go */*.go
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) cmd/dastard/dastard.go

# make test needs to install deps, or Travis will fail
test: deps
	$(GOFMT)
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME)

deps:
	$(GOGET) -v -t ./...

install: build
	cp -p $(BINARY_NAME) `go env GOPATH`/bin/

# EXPERIMENTAL: build a statically linked dastard binary with "make static".
# make static will _always_ rebuild the binary, and always with static linking
# The magic below won't make static binaries on Mac OS X (Darwin) at this time, so error on Macs.
STATICLDFLAGS=-ldflags "-linkmode external -extld g++ -extldflags '-static -lsodium'"
OS_NAME := $(shell uname -s | tr A-Z a-z)
static:
ifeq ($(OS_NAME),darwin)
	$(error Cannot build static binary on Mac OS)
endif
	$(GOBUILD) $(STATICLDFLAGS) $(LDFLAGS) -o $(BINARY_NAME) cmd/dastard/dastard.go
