.PHONY: build test run clean dry-run

BINARY := bin/tca-ovf-deployer
CONFIG ?= deploy.json

build:
	go build -o $(BINARY) .

test:
	go test ./...

run: build
	$(BINARY) -config $(CONFIG)

dry-run: build
	$(BINARY) -config $(CONFIG) -dry-run

clean:
	rm -rf bin logs
