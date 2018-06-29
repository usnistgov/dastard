GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=dastard

LDFLAGS=-ldflags "-X main.buildDate=$(shell date -u '+%Y-%m-%d.%I:%M:%S.%p.%Z') -X main.githash=$(shell git rev-parse HEAD)"
all: test build
build: $(BINARY_NAME)

$(BINARY_NAME): *.go cmd/dastard/dastard.go
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) cmd/dastard/dastard.go

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME)

deps:
	$(GOGET) github.com/markbates/goth
	$(GOGET) github.com/markbates/pop
