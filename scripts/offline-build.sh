#!/usr/bin/env bash
# Fully offline build + vet + test for evidencechain.
# The module has zero third-party dependencies.
set -euo pipefail
cd "$(dirname "$0")/.."

export GOPROXY=off
export GOSUMDB=off
export GOTOOLCHAIN=local
export CGO_ENABLED=0
export GOFLAGS=-mod=readonly

echo "== go version =="
go version
echo "== go build ./... =="
go build ./...
echo "== go vet ./... =="
go vet ./...
echo "== go test ./... =="
go test ./...
mkdir -p bin
go build -o bin/evidencechain ./cmd/evidencechain
echo "OFFLINE BUILD/TEST OK -> bin/evidencechain"
