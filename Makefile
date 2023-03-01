GOCMD=go
GOFMT=$(GOCMD) fmt
GOGET=$(GOCMD) get
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=dastard

LDFLAGS=-ldflags "-X main.buildDate=$(shell date -u '+%Y-%m-%d.%H:%M:%S.%Z') -X main.githash=$(shell git rev-parse --short HEAD)"
build: $(BINARY_NAME)
all: test build install

$(BINARY_NAME): *.go cmd/dastard/dastard.go getbytes/*.go lancero/*.go ljh/*.go off/*.go packets/*.go internal/*/*.go
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
