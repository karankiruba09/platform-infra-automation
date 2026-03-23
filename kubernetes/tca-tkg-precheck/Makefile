.PHONY: build test run clean

GO ?= go
BINARY_NAME ?= precheck

build:
	mkdir -p bin
	$(GO) build -o bin/$(BINARY_NAME) ./cmd/precheck

test:
	$(GO) test ./...

run:
	$(GO) run ./cmd/precheck

clean:
	rm -rf bin
