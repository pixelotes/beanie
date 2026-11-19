BIN := beanie
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test lint fmt run clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN) .

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

run: build
	./$(BIN) -transport http -addr 127.0.0.1:8090

clean:
	rm -f $(BIN)
