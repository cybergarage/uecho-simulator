#!/bin/sh
set -eu
export GOWORK=off GOPROXY=off GOTOOLCHAIN=local
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then printf '%s\n' "$unformatted"; exit 1; fi
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
mkdir -p bin
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/uecho-simulator-darwin-arm64 ./cmd/uecho-simulator
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/uecho-simulator-linux-arm64 ./cmd/uecho-simulator
make help
go run ./cmd/uecho-simulator --help >/dev/null
make demo </dev/null
make export </dev/null
