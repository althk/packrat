.PHONY: build test test-integration lint run install clean release completions

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/packrat ./cmd/packrat

test:
	go test -v -race -count=1 ./...

test-integration:
	go test -v -race -count=1 -tags=integration ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

run:
	go run ./cmd/packrat

install: build
	cp bin/packrat /usr/local/bin/

clean:
	rm -rf bin/ coverage.out coverage.html

release:
	goreleaser release --clean

completions: build
	bin/packrat completion bash > scripts/completions/packrat.bash
	bin/packrat completion zsh > scripts/completions/packrat.zsh
	bin/packrat completion fish > scripts/completions/packrat.fish
