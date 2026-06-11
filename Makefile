.PHONY: test vet build fmt race check

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

build:
	CGO_ENABLED=0 go build ./cmd/loopy

fmt:
	gofmt -w cmd internal

check: fmt vet test build
