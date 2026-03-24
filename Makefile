.PHONY: build test fmt vet clean

build:
	go build ./cmd/...

test:
	go test ./... -v

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	go clean
	rm -rf bin/
