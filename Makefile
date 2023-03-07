GOCMD=go
GOFMT=$(GOCMD) fmt
GOGET=$(GOCMD) get
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=dastard
STATIC_NAME=dastard_static

# The following uses the pure-Go "net" package netgo, instead of the usual link against C libraries.
# Added March 7, 2023 to make the Dastard binary more portable. But you can change to "NETGO=" to
# go back to the old way, if it seems useful.
NETGO=-tags netgo

.PHONY: all build install test clean run deps static

GLOBALVARIABLES=-X main.buildDate=$(shell date -u '+%Y-%m-%d.%H:%M:%S.%Z') -X main.githash=$(shell git rev-parse --short HEAD)
LDFLAGS=-ldflags "$(GLOBALVARIABLES)"
build: $(BINARY_NAME)
all: test build install

$(BINARY_NAME): Makefile *.go cmd/dastard/dastard.go */*.go
	$(GOBUILD) $(LDFLAGS) $(NETGO) -o $(BINARY_NAME) cmd/dastard/dastard.go

# make test needs to install deps, or Travis will fail
test: deps
	$(GOFMT)
	$(GOTEST) $(NETGO) -v ./...

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
STATICLDFLAGS=-ldflags "-linkmode external -extld g++ -extldflags '-static -lsodium' $(GLOBALVARIABLES)"
OS_NAME := $(shell uname -s | tr A-Z a-z)
static: $(STATIC_NAME)

$(STATIC_NAME): Makefile *.go cmd/dastard/dastard.go */*.go
ifeq ($(OS_NAME),darwin)
	$(error Cannot build static binary on Mac OS)
endif
	$(GOBUILD) $(STATICLDFLAGS) $(NETGO) -o $(BINARY_NAME) cmd/dastard/dastard.go
