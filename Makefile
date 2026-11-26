.PHONY: help build test vet offline verify report case clean

export GOPROXY ?= off
export GOSUMDB ?= off
export GOTOOLCHAIN ?= local

help:
	@echo "targets: build test vet offline verify report case clean"

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

offline:
	bash scripts/offline-build.sh

verify:
	go run ./cmd/evidencechain verify --case case --write-json case/out/verification.json

report:
	go run ./cmd/evidencechain report --case case --out FORENSIC-REPORT.md

case:
	bash scripts/rebuild-case.sh

clean:
	rm -rf bin evidencechain
