.PHONY: build test vet check

build:
	go build -o bin/hdr-sdr-manifest .

test:
	go test ./...

vet:
	go vet ./...

check: test vet build
