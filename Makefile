GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=dastard

LDFLAGS=-ldflags "-X main.buildDate=$(shell date -u '+%Y-%m-%d.%I:%M:%S.%p.%Z') -X main.githash=$(shell git rev-parse HEAD)"
all: test build
build: $(BINARY_NAME)

$(BINARY_NAME): *.go cmd/dastard/dastard.go getbytes/*.go lancero/*.go ljh/*.go off/*.go packets/*.go ringbuffer/*.go
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) cmd/dastard/dastard.go

# make test needs to install deps, or Travis will fail
test: deps
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME)

deps:
	$(GOGET) -v -t ./...

install: deps
