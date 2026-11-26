.PHONY: build test vet fmt demo example clean

build:
	go build -o framedoctor .

test:
	go test -v ./...

vet:
	go vet ./...

fmt:
	gofmt -w *.go

example: build
	./framedoctor gen --out examples/scenario
	./framedoctor analyze examples/scenario --all-windows

demo: build
	./framedoctor demo

clean:
	rm -f framedoctor
