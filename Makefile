BIN := beanie
LDFLAGS := -s -w

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
