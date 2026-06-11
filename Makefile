.PHONY: test vet build fmt race check tui-smoke dist

test:
	go test ./...

tui-smoke:
	CGO_ENABLED=0 go build -o /tmp/loopy-tui-smoke-bin ./cmd/loopy
	expect scripts/tui-smoke.exp /tmp/loopy-tui-smoke-bin

dist:
	scripts/dist.sh

race:
	go test -race ./...

vet:
	go vet ./...

build:
	CGO_ENABLED=0 go build ./cmd/loopy

fmt:
	gofmt -w cmd internal

check: fmt vet test build
