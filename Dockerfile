# syntax=docker/dockerfile:1
#
# Here is a demonstration Dockerfile for building Dastard and installing its variants.
# Not sure we want to use Docker, but this can help us get started.
#
FROM golang:1.24.5 AS build-stage
# RUN apk add --no-cache libzmq zeromq-dev
RUN apt-get update && apt-get install -y libsodium-dev libczmq-dev

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code. Note the slash at the end, as explained in
# https://docs.docker.com/reference/dockerfile/#copy
COPY . ./

# Build
# To do: set time zone and host name in the container from the host.
RUN GOARCH=$(uname -m) GOOS=linux && go build -o /bahama cmd/bahama/bahama.go
RUN export DATE_VAR=$(date -u '+%a, %e %b %Y %H:%M:%S %z') && \
    export GITHASH=$(git rev-parse --short HEAD) && \
    export GITDATE=$(git log -1 --pretty=format:"%ad" --date=format:"%a, %e %b %Y %H:%M:%S %z") && \
    GOARCH=$(uname -m) GOOS=linux && \
    go build -ldflags "-X 'main.buildDate=${DATE_VAR}' -X main.githash=${GITHASH} -X 'main.gitdate=${GITDATE}'" -tags netgo -o /dastard cmd/dastard/dastard.go


FROM build-stage AS run-test-stage
COPY internal/lancero/test_data/* ./internal/lancero/test_data/
COPY maps/* ./maps/
RUN groupadd -r myuser && useradd -r -g myuser myuser
RUN mkdir /home/myuser && chown -R myuser:myuser /home/myuser
RUN chown -R myuser: /app
USER myuser
RUN go test  ./...


FROM debian:latest AS build-release-stage
RUN apt-get update && apt-get install -y libsodium-dev libczmq-dev file less

# Couldn't get the compiled binary to work on Alpine linux. The instructions would have been:
# FROM alpine AS build-release-stage
# RUN apk add --no-cache libzmq zeromq-dev

WORKDIR /app
COPY --from=build-stage /dastard .
COPY --from=build-stage /bahama .

# Optional:
# To bind to a TCP port, runtime parameters must be supplied to the docker command.
# But we can document in the Dockerfile what ports
# the application is going to listen on by default.
# https://docs.docker.com/reference/dockerfile/#expose
EXPOSE 5500 5501 5502 5503 5504

ENTRYPOINT ["./dastard"]
